package saver

import (
	"os"
	"path/filepath"
	"testing"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/stretchr/testify/require"
)

func TestCodeSaver_SaveHtmlAndMultiFile(t *testing.T) {
	saver, err := NewCodeSaver()
	require.NoError(t, err)

	t.Run("html", func(t *testing.T) {
		dir, err := saver.SaveHtml(987654321, &aimodel.HtmlCodeResponse{
			HtmlCode:    "<main>hello</main>",
			Description: "test page",
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		require.Equal(t, dir, filepath.Join(saver.baseDir, "html_987654321"))
		require.FileExists(t, filepath.Join(dir, "index.html"))
		require.FileExists(t, filepath.Join(dir, "description.md"))
	})

	t.Run("multi file", func(t *testing.T) {
		dir, err := saver.SaveMultiFile(987654322, &aimodel.MultiFileCodeResponse{
			HtmlCodeResponse: aimodel.HtmlCodeResponse{
				HtmlCode:    "<main>hello</main>",
				Description: "test page",
			},
			CssCode: "main { color: red; }",
			JsCode:  "console.log('ok')",
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		require.Equal(t, dir, filepath.Join(saver.baseDir, "multi_file_987654322"))
		require.FileExists(t, filepath.Join(dir, "index.html"))
		require.FileExists(t, filepath.Join(dir, "style.css"))
		require.FileExists(t, filepath.Join(dir, "script.js"))
		require.FileExists(t, filepath.Join(dir, "description.md"))
	})

	t.Run("invalid app id", func(t *testing.T) {
		_, err := saver.GetDirPath(enum.HtmlCodeGen, 0)
		require.Error(t, err)
	})
}
