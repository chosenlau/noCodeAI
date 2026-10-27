package execution

import (
	"context"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceTypeUsesActualFiles(t *testing.T) {
	for _, tc := range []struct {
		name             string
		files            map[string]string
		configured, want enum.CodeGenTypeEnum
		fail             bool
	}{
		{"empty", nil, enum.VueCodeGen, "", false},
		{"description only", map[string]string{"description.md": "draft"}, enum.HtmlCodeGen, "", false},
		{"html", map[string]string{"index.html": "page"}, enum.VueCodeGen, enum.HtmlCodeGen, false},
		{"multi", map[string]string{"index.html": "page", "site.css": "css"}, enum.HtmlCodeGen, enum.MultiFileGen, false},
		{"vue", map[string]string{"package.json": "{}"}, enum.HtmlCodeGen, enum.VueCodeGen, false},
		{"unknown", map[string]string{"notes.txt": "do not erase"}, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, data := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(data), 0600))
			}
			got, nonempty, err := DetectSourceType(dir, tc.configured)
			if tc.fail {
				require.Error(t, err)
				require.True(t, nonempty)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func TestSourceHashPreservesBinaryAssetsAndExcludesBuildOutput(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "index.html"), []byte("page"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "photo.png"), []byte{0, 1, 255, 3}, 0600))
	first, err := SourceHash(ctx, source)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(source, "dist"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "dist", "built.js"), []byte("build"), 0600))
	withBuild, err := SourceHash(ctx, source)
	require.NoError(t, err)
	require.Equal(t, first, withBuild)
	copyDir := filepath.Join(t.TempDir(), "copy")
	require.NoError(t, PrepareWorkspace(ctx, copyDir, source))
	copied, err := SourceHash(ctx, copyDir)
	require.NoError(t, err)
	require.Equal(t, first, copied)
	require.NoError(t, os.WriteFile(filepath.Join(copyDir, "photo.png"), []byte{0, 1, 255, 4}, 0600))
	changed, err := SourceHash(ctx, copyDir)
	require.NoError(t, err)
	require.NotEqual(t, first, changed)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = SourceHash(canceled, source)
	require.ErrorIs(t, err, context.Canceled)
}
