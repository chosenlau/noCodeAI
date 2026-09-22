package agent

import (
	"context"
	"encoding/json"
	"fmt"

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
		return aimodel.QualityResult{IsValid: true}, err
	}

	return parseQualityResult(result.Content)
}

func parseQualityResult(content string) (aimodel.QualityResult, error) {
	var result struct {
		IsValid     bool     `json:"is_valid"`
		Errors      []string `json:"errors"`
		Suggestions []string `json:"suggestions"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return aimodel.QualityResult{
			IsValid:     true,
			Errors:      []string{"解析检查结果失败"},
			Suggestions: []string{fmt.Sprintf("原始响应: %s", content)},
		}, nil
	}

	return aimodel.QualityResult{
		IsValid:     result.IsValid,
		Errors:      result.Errors,
		Suggestions: result.Suggestions,
	}, nil
}
