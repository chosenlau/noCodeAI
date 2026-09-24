package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/chosenlau/noCodeAI/internal/ai/agent/agentmiddleware"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type HtmlCodeGenAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}
type MultiFileCodeGenAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}
type VueCodeGenAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

// NewHtmlCodeGenAgent 创建 HTML 代码生成 Agent 单例
func NewHtmlCodeGenAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
	agentMiddeleware *agentmiddleware.AgentMiddleware,
) *HtmlCodeGenAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, agentMiddeleware)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	adkAgent := baseAgent.NewAdkAgent(
		"HtmlFileCodeGenAgent",
		"A html code generator with strong code generation capabilities",
		prompt.GetHtmlPrompt(),
		[]tool.BaseTool{},
	)

	return &HtmlCodeGenAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

// NewMultiFileCodeGenAgent 创建多文件代码生成 Agent 单例
func NewMultiFileCodeGenAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
	agentMiddeleware *agentmiddleware.AgentMiddleware,
) *MultiFileCodeGenAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, agentMiddeleware)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	adkAgent := baseAgent.NewAdkAgent(
		"MultiFileCodeGenAgent",
		"A multifile code generator with strong code generation capabilities",
		prompt.GetMultiFilePrompt(),
		[]tool.BaseTool{},
	)

	return &MultiFileCodeGenAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

// NewVueCodeGenAgent 创建 Vue 代码生成 Agent 单例
func NewVueCodeGenAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
	toolManager *aitools.ToolManager,
	agentMiddeleware *agentmiddleware.AgentMiddleware,
) *VueCodeGenAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, agentMiddeleware)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	var tools []tool.BaseTool
	if toolManager != nil {
		tools = toolManager.GetAllTools()
	}

	adkAgent := baseAgent.NewAdkAgent(
		"VueCodeGenAgent",
		"A vue code generator with strong code generation capabilities",
		prompt.GetVuePrompt(),
		tools,
	)

	return &VueCodeGenAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

func (a *HtmlCodeGenAgent) GenerateHtmlCode(ctx context.Context, msg []*schema.Message) (*aimodel.HtmlCodeResponse, error) {
	message, err := a.Generate(ctx, msg, a.adkAgent)
	if err != nil {
		return nil, err
	}

	var result aimodel.HtmlCodeResponse
	parsedContent := ParseCodeResponse([]byte(message.Content))
	err = json.Unmarshal([]byte(parsedContent), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *MultiFileCodeGenAgent) GenerateMultiFileCode(ctx context.Context, msg []*schema.Message) (*aimodel.MultiFileCodeResponse, error) {
	message, err := a.Generate(ctx, msg, a.adkAgent)
	if err != nil {
		return nil, err
	}

	var result aimodel.MultiFileCodeResponse
	parsedContent := ParseCodeResponse([]byte(message.Content))
	err = json.Unmarshal([]byte(parsedContent), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *HtmlCodeGenAgent) GenerateHtmlCodeStream(ctx context.Context, msg []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	return a.GenerateStream(ctx, msg, a.adkAgent)
}

func (a *MultiFileCodeGenAgent) GenerateMultiFileCodeStream(ctx context.Context, msg []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	return a.GenerateStream(ctx, msg, a.adkAgent)
}

func (a *VueCodeGenAgent) GenerateVueProjectCodeStream(ctx context.Context, msg []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	return a.GenerateStream(ctx, msg, a.adkAgent)
}

func ParseCodeResponse(raw []byte) []byte {
	cleaned := bytes.TrimSpace(raw)

	if object := extractJSONObject(cleaned); len(object) > 0 {
		return normalizeCodeResponseJSON(object)
	}

	blocks := codeFenceRegex.FindAllSubmatch(cleaned, -1)
	if len(blocks) > 0 {
		fields := make(map[string]string)
		for _, block := range blocks {
			kind := strings.ToLower(string(block[1]))
			value := strings.TrimSpace(string(block[2]))
			switch kind {
			case "html", "vue", "component":
				fields["html_code"] = value
			case "css":
				fields["css_code"] = value
			case "js", "javascript", "typescript", "ts":
				fields["js_code"] = value
			case "description", "markdown", "md":
				fields["description"] = value
			}
		}
		if len(fields) > 0 {
			return marshalCodeResponse(fields)
		}
	}

	if bytes.Contains(cleaned, []byte("```")) {
		fallback := bytes.ReplaceAll(cleaned, []byte("```"), nil)
		lines := bytes.SplitN(fallback, []byte("\n"), 2)
		if len(lines) == 2 && isCodeFenceLanguage(bytes.TrimSpace(lines[0])) {
			fallback = lines[1]
		}
		if object := extractJSONObject(fallback); len(object) > 0 {
			return normalizeCodeResponseJSON(object)
		}

		firstFence := bytes.Index(cleaned, []byte("```"))
		if firstFence >= 0 {
			body := cleaned[firstFence+3:]
			if newline := bytes.IndexByte(body, '\n'); newline >= 0 {
				kind := strings.ToLower(strings.TrimSpace(string(body[:newline])))
				body = bytes.TrimSpace(body[newline+1:])
				body = bytes.TrimSpace(bytes.TrimSuffix(body, []byte("```")))
				fields := map[string]string{}
				switch kind {
				case "html", "vue", "component", "":
					fields["html_code"] = string(body)
				case "css":
					fields["css_code"] = string(body)
				case "js", "javascript", "typescript", "ts":
					fields["js_code"] = string(body)
				}
				if len(fields) > 0 {
					return marshalCodeResponse(fields)
				}
			}
		}
	}

	if html := extractRawHTML(cleaned); len(html) > 0 {
		return marshalCodeResponse(map[string]string{
			"html_code": string(html),
		})
	}

	return cleaned
}

func extractRawHTML(raw []byte) []byte {
	if bytes.Contains(raw, []byte("```")) {
		return nil
	}

	lower := strings.ToLower(string(raw))
	candidates := []string{"<!doctype html", "<html", "<main", "<body", "<section", "<div"}
	start := -1
	for _, candidate := range candidates {
		if index := strings.Index(lower, candidate); index >= 0 && (start < 0 || index < start) {
			start = index
		}
	}
	if start < 0 {
		return nil
	}

	html := bytes.TrimSpace(raw[start:])
	if !bytes.Contains(bytes.ToLower(html), []byte("</html>")) &&
		!bytes.Contains(bytes.ToLower(html), []byte("</main>")) &&
		!bytes.Contains(bytes.ToLower(html), []byte("</body>")) &&
		!bytes.Contains(bytes.ToLower(html), []byte("</section>")) &&
		!bytes.Contains(bytes.ToLower(html), []byte("</div>")) {
		return nil
	}
	return html
}

var codeFenceRegex = regexp.MustCompile("(?s)```[ \\t]*([A-Za-z0-9_-]*)[ \\t]*\\r?\\n(.*?)```")

func extractJSONObject(raw []byte) []byte {
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		return nil
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(raw); i++ {
		switch raw[i] {
		case '\\':
			if inString {
				escaped = !escaped
			}
		case '"':
			if !escaped {
				inString = !inString
			}
			escaped = false
		default:
			escaped = false
		}
		if inString {
			continue
		}
		switch raw[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := raw[start : i+1]
				if json.Valid(candidate) {
					return candidate
				}
				start = bytes.IndexByte(raw[i+1:], '{')
				if start < 0 {
					return nil
				}
				start += i + 1
				depth = 0
			}
		}
	}
	return nil
}

func normalizeCodeResponseJSON(raw []byte) []byte {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return bytes.TrimSpace(raw)
	}

	normalized := make(map[string]any, len(value))
	for key, item := range value {
		switch strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_")) {
		case "html", "htmlcode", "html_code", "template", "content":
			normalized["html_code"] = item
		case "css", "csscode", "css_code", "style", "style_code":
			normalized["css_code"] = item
		case "js", "javascript", "jscode", "js_code", "script", "script_code":
			normalized["js_code"] = item
		case "description", "desc", "summary":
			normalized["description"] = item
		default:
			normalized[key] = item
		}
	}

	return marshalCodeResponse(normalized)
}

func marshalCodeResponse(value any) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(value)
	if err != nil {
		return nil
	}
	return bytes.TrimSpace(buffer.Bytes())
}

func isCodeFenceLanguage(value []byte) bool {
	for _, b := range value {
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && (b < '0' || b > '9') {
			return false
		}
	}
	return true
}
