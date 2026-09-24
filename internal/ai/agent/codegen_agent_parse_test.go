package agent

import (
	"encoding/json"
	"testing"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/stretchr/testify/require"
)

func TestParseCodeResponseVariants(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantHTML string
		wantCSS  string
		wantJS   string
		wantDesc string
	}{
		{
			name:     "plain json with unicode",
			response: `{"html_code":"<h1>你好</h1>","description":"中文页面"}`,
			wantHTML: "<h1>你好</h1>",
			wantDesc: "中文页面",
		},
		{
			name:     "json code fence",
			response: "```json\n{\"html_code\":\"<main>ok</main>\",\"description\":\"test\"}\n```",
			wantHTML: "<main>ok</main>",
			wantDesc: "test",
		},
		{
			name:     "html code fence",
			response: "```html\n<!doctype html><html lang=\"zh-CN\"><body>你好</body></html>\n```",
			wantHTML: "<!doctype html><html lang=\"zh-CN\"><body>你好</body></html>",
		},
		{
			name:     "raw html with explanatory prefix",
			response: "Generated page:\n<!doctype html><html><body>ok</body></html>",
			wantHTML: "<!doctype html><html><body>ok</body></html>",
		},
		{
			name:     "multi file code fences",
			response: "```html\n<div>页面</div>\n```\n```css\nbody { color: red; }\n```\n```javascript\nconsole.log('ok')\n```",
			wantHTML: "<div>页面</div>",
			wantCSS:  "body { color: red; }",
			wantJS:   "console.log('ok')",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result aimodel.MultiFileCodeResponse
			parsed := ParseCodeResponse([]byte(test.response))
			require.NoError(t, json.Unmarshal(parsed, &result), string(parsed))
			require.Equal(t, test.wantHTML, result.HtmlCode)
			require.Equal(t, test.wantCSS, result.CssCode)
			require.Equal(t, test.wantJS, result.JsCode)
			require.Equal(t, test.wantDesc, result.Description)
		})
	}
}

func TestParseCodeResponseRejectsMalformedJSON(t *testing.T) {
	parsed := ParseCodeResponse([]byte("```json\n{\"html_code\":\"<div>坏掉了</div>\" ä}\n```"))

	var result aimodel.HtmlCodeResponse
	require.Error(t, json.Unmarshal(parsed, &result))
}
