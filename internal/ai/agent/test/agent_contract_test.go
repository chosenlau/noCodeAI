package agent_test

import (
	"bytes"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/stretchr/testify/require"
)

func TestParseCodeResponse_RemovesMarkdownFence(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "json fence",
			raw:  "  ```json\n{\"html_code\":\"<h1>Hello</h1>\"}\n```  ",
			want: `{"html_code":"<h1>Hello</h1>"}`,
		},
		{
			name: "plain fence",
			raw:  "```html\n<h1>Hello</h1>\n```",
			want: `{"html_code":"<h1>Hello</h1>"}`,
		},
		{
			name: "plain json",
			raw:  `{"html_code":"<h1>Hello</h1>"}`,
			want: `{"html_code":"<h1>Hello</h1>"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, string(agent.ParseCodeResponse([]byte(tt.raw))))
		})
	}
}

func TestParseCodeResponse_PreservesInnerFenceContent(t *testing.T) {
	raw := "```json\n{\"html_code\":\"```<h1>Hello</h1>```\"}\n```"
	got := agent.ParseCodeResponse([]byte(raw))

	require.True(t, bytes.HasPrefix(got, []byte(`{"html_code":"`)))
	require.True(t, bytes.HasSuffix(got, []byte(`"}`)))
}

func TestParseCodeResponse_NormalizesCommonLLMFormats(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "camel case json with explanation",
			raw:  "结果如下：\n{\"htmlCode\":\"<main/>\",\"cssCode\":\"main{}\",\"jsCode\":\"console.log(1)\"}",
			want: `{"css_code":"main{}","html_code":"<main/>","js_code":"console.log(1)"}`,
		},
		{
			name: "markdown code blocks",
			raw:  "```html\n<main>Hello</main>\n```\n```css\nmain { color: red; }\n```\n```javascript\nconsole.log('ok')\n```",
			want: `{"css_code":"main { color: red; }","html_code":"<main>Hello</main>","js_code":"console.log('ok')"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.JSONEq(t, tt.want, string(agent.ParseCodeResponse([]byte(tt.raw))))
		})
	}
}

func TestModelContractsCanBeConstructed(t *testing.T) {
	response := aimodel.HtmlCodeResponse{
		HtmlCode:    "<main>Hello</main>",
		Description: "test page",
	}

	require.Equal(t, "<main>Hello</main>", response.HtmlCode)
	require.Equal(t, "test page", response.Description)
}
