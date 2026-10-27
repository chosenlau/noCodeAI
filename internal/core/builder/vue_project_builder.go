package builder

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
)

type Result struct{ Directory, Dist, Log string }

// BuildProjectContext builds a disposable copy without inheriting backend credentials.
// It is not an OS sandbox. The caller owns successful artifacts and their retention.
func BuildProjectContext(ctx context.Context, source string) (result Result, err error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	root, err := os.MkdirTemp("", "nocode-build-")
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(root)
		}
	}()
	result.Directory = root
	project := filepath.Join(root, "project")
	if err = execution.PrepareWorkspace(ctx, project, source); err != nil {
		return result, err
	}
	if _, err = os.Stat(filepath.Join(project, "package.json")); err != nil {
		return result, err
	}
	for _, name := range []string{"home", "tmp", "cache"} {
		if err = os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			return result, err
		}
	}
	for _, name := range []string{"user.npmrc", "global.npmrc"} {
		if err = os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			return result, err
		}
	}
	node, cli, err := npmExecutable()
	if err != nil {
		return result, err
	}
	log := &boundedLog{}
	defer func() { result.Log = log.String() }()
	install := "install"
	if _, statErr := os.Stat(filepath.Join(project, "package-lock.json")); statErr == nil {
		install = "ci"
	}
	base := []string{cli, "--userconfig", filepath.Join(root, "user.npmrc"), "--globalconfig", filepath.Join(root, "global.npmrc"), "--cache", filepath.Join(root, "cache"), "--ignore-scripts"}
	for _, args := range [][]string{{install, "--no-audit", "--no-fund"}, {"run", "build"}} {
		cmd := exec.Command(node, append(append([]string{}, base...), args...)...)
		cmd.Dir = project
		cmd.Env = buildEnvironment(root)
		cmd.Stdout, cmd.Stderr = log, log
		if err = runProcess(ctx, cmd); err != nil {
			return result, fmt.Errorf("website build failed: %w", err)
		}
	}
	result.Dist = filepath.Join(project, "dist")
	info, statErr := os.Lstat(result.Dist)
	if statErr != nil {
		return result, statErr
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("build produced no regular dist directory")
	}
	return result, nil
}

func npmExecutable() (string, string, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return "", "", err
	}
	npmName := "npm"
	if runtime.GOOS == "windows" {
		npmName = "npm.cmd"
	}
	npm, err := exec.LookPath(npmName)
	if err != nil {
		return "", "", err
	}
	resolved, err := filepath.EvalSymlinks(npm)
	if err != nil {
		return "", "", err
	}
	candidates := []string{filepath.Join(filepath.Dir(resolved), "node_modules", "npm", "bin", "npm-cli.js"), filepath.Join(filepath.Dir(resolved), "npm-cli.js"), filepath.Join(filepath.Dir(node), "node_modules", "npm", "bin", "npm-cli.js")}
	for _, cli := range candidates {
		if info, err := os.Stat(cli); err == nil && info.Mode().IsRegular() {
			return node, cli, nil
		}
	}
	return "", "", fmt.Errorf("npm-cli.js not found; install Node.js with npm")
}

func buildEnvironment(root string) []string {
	env := []string{}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "COMSPEC", "PATHEXT"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	home, tmp := filepath.Join(root, "home"), filepath.Join(root, "tmp")
	return append(env, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home, "TMP="+tmp, "TEMP="+tmp, "TMPDIR="+tmp, "CI=true", "NO_COLOR=1")
}

const maxLogBytes = 64 << 10

type boundedLog struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *boundedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := len(p)
	remaining := maxLogBytes - l.text.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = l.text.Write(p)
	}
	return n, nil
}
func (l *boundedLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.text.String() }

func runProcess(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	prepareProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	cleanup, err := containProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	defer cleanup()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		cleanup()
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}
