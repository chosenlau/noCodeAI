package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chosenlau/noCodeAI/pkg/enum"
)

// SourceHash includes every copied source/resource byte, including binary assets
// and descriptions. It is separate from the code-only map sent to the UI/LLM.
func SourceHash(ctx context.Context, directory string) (string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	type entry struct{ Path, Hash string }
	var entries []entry
	count, total := 0, int64(0)
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source contains symlink: %s", path)
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" {
				return fs.SkipDir
			}
			return nil
		}
		data, err := ReadFile(root, path)
		if err != nil {
			return err
		}
		count++
		total += int64(len(data))
		if count > MaxProjectFiles || total > MaxProjectBytes {
			return fmt.Errorf("source exceeds project budget")
		}
		hash := sha256.Sum256(data)
		entries = append(entries, entry{filepath.ToSlash(path), hex.EncodeToString(hash[:])})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	data, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// DetectSourceType uses actual source entries, never a model's interpretation
// of the next user request. Unknown nonempty projects fail rather than rebuild.
func DetectSourceType(directory string, configured enum.CodeGenTypeEnum) (enum.CodeGenTypeEnum, bool, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", false, err
	}
	defer root.Close()
	var paths []string
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("source contains symlink: %s", path)
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "dist" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(path, "description.md") {
			paths = append(paths, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, nil
	}
	has := func(name string) bool {
		for _, path := range paths {
			if path == name {
				return true
			}
		}
		return false
	}
	if has("package.json") {
		return enum.VueCodeGen, true, nil
	}
	if has("index.html") {
		if configured == enum.MultiFileGen {
			return configured, true, nil
		}
		for _, path := range paths {
			if ext := strings.ToLower(filepath.Ext(path)); ext == ".css" || ext == ".js" {
				return enum.MultiFileGen, true, nil
			}
		}
		return enum.HtmlCodeGen, true, nil
	}
	return "", true, fmt.Errorf("existing source has no supported project entry; refusing to replace it")
}
