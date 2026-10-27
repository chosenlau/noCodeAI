package execution

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Workspace belongs to one execution, never to a shared Agent instance.
type Workspace struct {
	AppID       int64
	ExecutionID string
	Root        string
	mu          sync.Mutex
}

const MaxFileBytes int64 = 1 << 20
const MaxProjectBytes int64 = 20 << 20
const MaxProjectFiles = 500

func (w *Workspace) Lock() func() { w.mu.Lock(); return w.mu.Unlock }

func CheckWrite(root *os.Root, name string, size int64) error {
	if size > MaxFileBytes {
		return fmt.Errorf("file exceeds %d byte limit", MaxFileBytes)
	}
	count, total := 0, int64(0)
	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace contains symlink: %s", path)
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" || entry.Name() == "dist" {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Clean(filepath.FromSlash(path)) == filepath.Clean(name) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular workspace file")
		}
		count++
		total += info.Size()
		if count >= MaxProjectFiles || total+size > MaxProjectBytes {
			return fmt.Errorf("project file/size budget exceeded")
		}
		return nil
	})
	return err
}

func ReadFile(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
		return nil, fmt.Errorf("file is not regular or exceeds size budget")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxFileBytes {
		return nil, fmt.Errorf("file exceeds size budget")
	}
	return data, nil
}

type workspaceKey struct{}

func WithWorkspace(ctx context.Context, workspace *Workspace) context.Context {
	return context.WithValue(ctx, workspaceKey{}, workspace)
}
func WorkspaceFromContext(ctx context.Context) (*Workspace, bool) {
	w, ok := ctx.Value(workspaceKey{}).(*Workspace)
	return w, ok && w != nil
}

// RelativePath accepts only relative project paths. Actual IO must use os.Root,
// which also prevents symlink/junction races while resolving paths.
func RelativePath(name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.Contains(name, ":") || filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("absolute or device path is forbidden")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}
	return clean, nil
}

func ResolvePath(root, name string) (string, error) {
	rel, err := RelativePath(name)
	if err != nil {
		return "", err
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, rel), nil
}

// PrepareWorkspace copies regular source files, never hard links or symlinks.
// Root must be a fresh directory so a retry cannot silently reuse stale files.
func PrepareWorkspace(ctx context.Context, root, source string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(root); err == nil {
		return fmt.Errorf("workspace already exists: %s", root)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	if source == "" {
		return nil
	}
	src, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dst.Close()
	count, total := 0, int64(0)
	return fs.WalkDir(src.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source contains symlink: %s", path)
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == ".git" || entry.Name() == "dist" {
				return fs.SkipDir
			}
			return dst.MkdirAll(path, 0755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source contains non-regular file: %s", path)
		}
		count++
		total += info.Size()
		if count > MaxProjectFiles || total > MaxProjectBytes {
			return fmt.Errorf("project file/size budget exceeded")
		}
		data, err := ReadFile(src, path)
		if err != nil {
			return err
		}
		return dst.WriteFile(path, data, 0644)
	})
}

// Freeze creates a separate, unpublished version directory. No tools receive
// its path. Publishing the prepared directory remains the caller's DB decision.
func Freeze(ctx context.Context, workspace *Workspace, versionPath string) (string, error) {
	if workspace == nil {
		return "", fmt.Errorf("workspace is missing")
	}
	if err := PrepareWorkspace(ctx, versionPath, workspace.Root); err != nil {
		return "", err
	}
	return filepath.Abs(versionPath)
}
