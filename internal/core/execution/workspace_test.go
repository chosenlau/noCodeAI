package execution

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspacePrepareAndFreezeAreIndependent(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "source")
	require.NoError(t, os.Mkdir(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "index.html"), []byte("published"), 0644))
	work := filepath.Join(base, "work")
	require.NoError(t, PrepareWorkspace(context.Background(), work, source))
	require.NoError(t, os.WriteFile(filepath.Join(work, "index.html"), []byte("draft"), 0644))
	data, err := os.ReadFile(filepath.Join(source, "index.html"))
	require.NoError(t, err)
	require.Equal(t, "published", string(data))
	version, err := Freeze(context.Background(), &Workspace{Root: work}, filepath.Join(base, "version"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(work, "index.html"), []byte("later"), 0644))
	data, err = os.ReadFile(filepath.Join(version, "index.html"))
	require.NoError(t, err)
	require.Equal(t, "draft", string(data))
	require.Error(t, PrepareWorkspace(context.Background(), work, source))
}

func TestWorkspaceRejectsEscapes(t *testing.T) {
	for _, p := range []string{"../secret", `..\secret`, "/tmp/secret", `C:\secret`, "file:stream", `\\host\share`} {
		_, err := RelativePath(p)
		require.Error(t, err, p)
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink privilege unavailable: %v", err)
	}
	r, err := os.OpenRoot(root)
	require.NoError(t, err)
	defer r.Close()
	require.Error(t, r.WriteFile(filepath.Join("escape", "secret"), []byte("bad"), 0644))
	require.NoFileExists(t, filepath.Join(outside, "secret"))
	require.Error(t, PrepareWorkspace(context.Background(), filepath.Join(t.TempDir(), "copy"), root))
}
