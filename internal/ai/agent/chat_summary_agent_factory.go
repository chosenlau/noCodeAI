package agent

import (
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
)

type ChatSummaryAgentFactory struct {
	chatModel        ChatModelWrapperAdaptor
	checkpointStore  adk.CheckPointStore
	metricsCollector *monitor.AiModelMetricsCollector
}

func NewChatSummaryAgentFactory(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
) *ChatSummaryAgentFactory {
	return &ChatSummaryAgentFactory{
		chatModel:        chatModel,
		checkpointStore:  checkpointStore,
		metricsCollector: metricsCollector,
	}
}

func (f *ChatSummaryAgentFactory) GetChatSummaryAgent() *ChatSummaryAgent {
	return NewChatSummaryAgent(
		f.chatModel,
		f.checkpointStore,
		f.metricsCollector,
	)
}
