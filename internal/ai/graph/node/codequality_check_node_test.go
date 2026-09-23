package node

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/stretchr/testify/require"
)

func TestReadCodeFiles(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dist"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<main />"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src", "App.vue"), []byte("<template />"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"build":"vite"}}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("ignored"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "node_modules", "pkg", "ignored.js"), []byte("ignored"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "dist", "ignored.js"), []byte("ignored"), 0o644))

	files, _, err := ReadCodeFiles(root)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"index.html":   "<main />",
		"package.json": `{"scripts":{"build":"vite"}}`,
		"src/App.vue":  "<template />",
	}, files)
}

func TestConcatenateCodeFilesIsDeterministic(t *testing.T) {
	content := concatenateCodeFiles(map[string]string{
		"src/main.js": "main",
		"index.html":  "html",
	})

	require.Contains(t, content, "// ===== File: index.html =====")
	require.Contains(t, content, "// ===== File: src/main.js =====")
	require.Less(t,
		indexOf(t, content, "File: index.html"),
		indexOf(t, content, "File: src/main.js"),
	)
}

func TestCodeQualityCheckNodeStoresCodeContent(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<main />"), 0o644))

	input := &state.GraphState{
		WorkFlowContext: &state.WorkFlowContext{GenerateCodeDir: root},
	}
	result, err := (&CodeQualityCheckNode{}).execute(context.Background(), input)

	require.NoError(t, err)
	require.Same(t, input, result)
	require.Equal(t, map[string]string{"index.html": "<main />"}, result.WorkFlowContext.CodeContent)
	require.True(t, result.WorkFlowContext.QualityResult.IsValid)
}

func indexOf(t *testing.T, content, needle string) int {
	t.Helper()
	index := strings.Index(content, needle)
	require.NotEqual(t, -1, index)
	return index
}
