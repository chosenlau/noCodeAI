package aitools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileWriteToolCancelledBeforeWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fileWriteToolFunc(ctx, FileWriteToolParams{
		RelativePath: filepath.Join(t.TempDir(), "cancelled.txt"),
		Content:      "should not be written",
	})

	require.ErrorIs(t, err, context.Canceled)
}

func TestFileReadToolCancelledBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fileReadToolFunc(ctx, FileReadToolParams{
		RelativePath: filepath.Join(t.TempDir(), "missing.txt"),
	})

	require.ErrorIs(t, err, context.Canceled)
}

func TestFileModifyToolCancelledBeforeWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fileModifyToolFunc(ctx, FileModifyToolParams{
		RelativeFilePath: path,
		OldContent:       "original",
		NewContent:       "changed",
	})

	require.ErrorIs(t, err, context.Canceled)
	content, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, "original", string(content))
}

func TestFileDeleteToolCancelledBeforeDelete(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.txt")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fileDeleteToolFunc(ctx, FileDeleteToolParams{RelativePath: path})

	require.ErrorIs(t, err, context.Canceled)
	require.FileExists(t, path)
}
