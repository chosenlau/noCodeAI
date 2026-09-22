package agent

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type CodeGenTypeRoutingAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

func NewCodeGenTypeRoutingAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
) *CodeGenTypeRoutingAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, nil)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	adkAgent := baseAgent.NewAdkAgent(
		"代码生成类型路由器",
		"根据用户需求判断最合适的代码生成类型",
		prompt.GetRoutingPrompt(),
		[]tool.BaseTool{},
	)

	return &CodeGenTypeRoutingAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

func (a *CodeGenTypeRoutingAgent) RouteCodeGenType(ctx context.Context, messages []*schema.Message) (enum.CodeGenTypeEnum, error) {
	message, err := a.Generate(ctx, messages, a.adkAgent)
	if err != nil {
		return "", err
	}

	return enum.CodeGenTypeEnum(message.Content), nil
}
