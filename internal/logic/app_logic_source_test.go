package logic

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/stretchr/testify/require"
)

func TestFindSourceCodeDirectorySupportsAllGenerationTypes(t *testing.T) {
	for _, codeGenType := range []enum.CodeGenTypeEnum{
		enum.HtmlCodeGen,
		enum.MultiFileGen,
		enum.VueCodeGen,
	} {
		t.Run(string(codeGenType), func(t *testing.T) {
			root := t.TempDir()
			expected := filepath.Join(root, string(codeGenType)+"_123")
			require.NoError(t, os.MkdirAll(expected, 0755))

			actual, err := findSourceCodeDirectory(
				[]string{root},
				123,
				codeGenType,
				"",
			)

			require.NoError(t, err)
			require.Equal(t, expected, actual)
		})
	}
}

func TestFindSourceCodeDirectoryFallsBackToExistingDirectory(t *testing.T) {
	root := t.TempDir()
	expected := filepath.Join(root, "multi_file_456")
	require.NoError(t, os.MkdirAll(expected, 0755))

	actual, err := findSourceCodeDirectory(
		[]string{root},
		456,
		"",
		"",
	)

	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestFindSourceCodeDirectorySupportsLegacyRoot(t *testing.T) {
	currentRoot := t.TempDir()
	legacyRoot := t.TempDir()
	expected := filepath.Join(legacyRoot, "html_789")
	require.NoError(t, os.MkdirAll(expected, 0755))

	actual, err := findSourceCodeDirectory(
		[]string{currentRoot, legacyRoot},
		789,
		enum.HtmlCodeGen,
		"",
	)

	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
