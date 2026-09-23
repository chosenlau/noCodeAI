package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseQualityResultMarkdownJSON(t *testing.T) {
	result, err := parseQualityResult("```json\n{\"isValid\":true,\"errors\":[],\"suggestions\":[\"add a description\"]}\n```\n\nThe code is valid.")

	require.NoError(t, err)
	require.True(t, result.IsValid)
	require.Empty(t, result.Errors)
	require.Equal(t, []string{"add a description"}, result.Suggestions)
}

func TestParseQualityResultIgnoresMarkdownJSONExample(t *testing.T) {
	content := "```json\n{\"isValid\":true,\"errors\":[],\"suggestions\":[\"keep package.json\"]}\n```\n\n说明如下：\n```json\n{\"example\":true}\n```"

	result, err := parseQualityResult(content)

	require.NoError(t, err)
	require.True(t, result.IsValid)
	require.Equal(t, []string{"keep package.json"}, result.Suggestions)
}

func TestParseQualityResultInvalidResponse(t *testing.T) {
	result, err := parseQualityResult("not json")

	require.Error(t, err)
	require.False(t, result.IsValid)
}
