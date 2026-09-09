package parser

import (
	"regexp"
	"strings"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/ai_model"
)

var (
	htmlCodeRegex = regexp.MustCompile("(?i)```html\\s*\\n([\\s\\S]*?)```")
	cssCodeRegex  = regexp.MustCompile("(?i)```css\\s*\\n([\\s\\S]*?)```")
	jsCodeRegex   = regexp.MustCompile("(?i)```(?:js|javascript)\\s*\\n([\\s\\S]*?)```")
)

func ParseHtmlCode(codeContent string) *aimodel.HtmlCodeResponse {
	result := &aimodel.HtmlCodeResponse{}

	htmlCode := extractCodeByPattern(codeContent, htmlCodeRegex)
	if htmlCode != "" {
		result.HtmlCode = strings.TrimSpace(htmlCode)
	} else {
		result.HtmlCode = strings.TrimSpace(codeContent)
	}

	return result
}

func ParseMultiFileCode(codeContent string) *aimodel.MultiFileCodeResponse {
	result := &aimodel.MultiFileCodeResponse{}

	htmlCode := extractCodeByPattern(codeContent, htmlCodeRegex)
	cssCode := extractCodeByPattern(codeContent, cssCodeRegex)
	jsCode := extractCodeByPattern(codeContent, jsCodeRegex)

	if htmlCode != "" {
		result.HtmlCode = strings.TrimSpace(htmlCode)
	}

	if cssCode != "" {
		result.CssCode = strings.TrimSpace(cssCode)
	}

	if jsCode != "" {
		result.JsCode = strings.TrimSpace(jsCode)
	}

	return result
}

func extractCodeByPattern(content string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(content)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
