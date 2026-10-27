package builder

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildEnvironmentAndLogBound(t *testing.T) {
	t.Setenv("AI_API_KEY", "private-value")
	t.Setenv("NODE_OPTIONS", "--require private-server-module")
	env := strings.Join(buildEnvironment(t.TempDir()), "\n")
	require.NotContains(t, env, "private-value")
	require.NotContains(t, env, "NODE_OPTIONS")
	log := &boundedLog{}
	n, err := log.Write([]byte(strings.Repeat("x", maxLogBytes*2)))
	require.NoError(t, err)
	require.Equal(t, maxLogBytes*2, n)
	require.Len(t, log.String(), maxLogBytes)
}

func TestBuildProcessCancellation(t *testing.T) {
	if os.Getenv("NOCODE_BUILD_TEST_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBuildProcessCancellation$")
	cmd.Env = append(buildEnvironment(t.TempDir()), "NOCODE_BUILD_TEST_HELPER=1")
	started := time.Now()
	err := runProcess(ctx, cmd)
	require.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)
	require.Less(t, time.Since(started), 5*time.Second)
	require.NotNil(t, cmd.ProcessState)
}

func TestBuildCancellationTerminatesDescendants(t *testing.T) {
	switch os.Getenv("NOCODE_BUILD_TREE_HELPER") {
	case "child":
		time.Sleep(2 * time.Second)
		_ = os.WriteFile(os.Getenv("NOCODE_BUILD_MARKER"), []byte("survived"), 0600)
		os.Exit(0)
	case "parent":
		time.Sleep(100 * time.Millisecond)
		child := exec.Command(os.Args[0], "-test.run=^TestBuildCancellationTerminatesDescendants$")
		child.Env = append(os.Environ(), "NOCODE_BUILD_TREE_HELPER=child")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	marker := filepath.Join(t.TempDir(), "survived.txt")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBuildCancellationTerminatesDescendants$")
	cmd.Env = append(buildEnvironment(t.TempDir()), "NOCODE_BUILD_TREE_HELPER=parent", "NOCODE_BUILD_MARKER="+marker)
	require.ErrorIs(t, runProcess(ctx, cmd), context.DeadlineExceeded)
	time.Sleep(2500 * time.Millisecond)
	_, err := os.Stat(marker)
	require.True(t, os.IsNotExist(err), "descendant survived cancellation: %v", err)
}
