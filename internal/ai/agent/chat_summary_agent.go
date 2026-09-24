package agent

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ChatSummaryAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

func NewChatSummaryAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
) *ChatSummaryAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, nil)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	adkAgent := baseAgent.NewAdkAgent(
		"对话总结助手",
		"总结对话历史，提取关键信息和要点",
		prompt.GetChatSummaryPrompt(),
		[]tool.BaseTool{},
	)

	return &ChatSummaryAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

func (a *ChatSummaryAgent) SummarizeChat(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	result, err := a.Generate(ctx, messages, a.adkAgent)
	if err != nil {
		return nil, err
	}

	return result, nil
}
