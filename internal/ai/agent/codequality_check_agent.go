package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type CodeQualityCheckAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

func NewCodeQualityCheckAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
) *CodeQualityCheckAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, nil)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	adkAgent := baseAgent.NewAdkAgent(
		"代码质量检查助手",
		"检查代码质量，发现潜在问题并提供改进建议",
		prompt.GetCodeQualityCheckPrompt(),
		[]tool.BaseTool{},
	)

	return &CodeQualityCheckAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

func (a *CodeQualityCheckAgent) CheckCodeQuality(ctx context.Context, messages []*schema.Message) (aimodel.QualityResult, error) {
	result, err := a.Generate(ctx, messages, a.adkAgent)
	if err != nil {
		return aimodel.QualityResult{IsValid: false}, err
	}

	return parseQualityResult(result.Content)
}

func parseQualityResult(content string) (aimodel.QualityResult, error) {
	var result struct {
		IsValid     bool     `json:"isValid"`
		Errors      []string `json:"errors"`
		Suggestions []string `json:"suggestions"`
	}

	jsonContent := extractQualityJSON(content)
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return aimodel.QualityResult{IsValid: false}, fmt.Errorf(
			"parse quality result: %w; response: %s",
			err,
			content,
		)
	}

	return aimodel.QualityResult{
		IsValid:     result.IsValid,
		Errors:      result.Errors,
		Suggestions: result.Suggestions,
	}, nil
}

func extractQualityJSON(content string) string {
	content = strings.TrimSpace(content)
	for start := strings.Index(content, "{"); start >= 0; {
		candidate := content[start:]
		var value map[string]any
		decoder := json.NewDecoder(strings.NewReader(candidate))
		if err := decoder.Decode(&value); err == nil {
			end := start + int(decoder.InputOffset())
			return content[start:end]
		}
		next := strings.Index(candidate[1:], "{")
		if next < 0 {
			break
		}
		start += next + 1
	}
	return content
}
