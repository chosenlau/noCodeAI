package agent

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ChatAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

func NewChatAgent(chatModel ChatModelWrapperAdaptor, checkpointStore adk.CheckPointStore, metricsCollector *monitor.AiModelMetricsCollector) *ChatAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, nil)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}
	return &ChatAgent{
		BaseAgent: baseAgent,
		adkAgent: baseAgent.NewAdkAgent(
			"ChatAgent",
			"A conversational assistant for discussing ideas without generating source code",
			prompt.GetChatAgentPrompt(),
			[]tool.BaseTool{},
		),
	}
}

func (a *ChatAgent) Chat(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	return a.GenerateStream(ctx, messages, a.adkAgent)
}
