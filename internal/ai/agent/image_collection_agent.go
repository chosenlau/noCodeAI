package agent

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ImageCollectionAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

func NewImageCollectionAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
	imageSearchTool *aitools.ImageSearchTool,
	undrawIllustrationTool *aitools.UndrawIllustrationTool,
	mermaidDiagramTool *aitools.MermaidDiagramTool,
	logoGeneratorTool *aitools.LogoGeneratorTool,
) *ImageCollectionAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, nil)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	var tools []tool.BaseTool
	if imageSearchTool != nil {
		tools = append(tools, imageSearchTool.BaseTool)
	}
	if undrawIllustrationTool != nil {
		tools = append(tools, undrawIllustrationTool.BaseTool)
	}
	if mermaidDiagramTool != nil {
		tools = append(tools, mermaidDiagramTool.BaseTool)
	}
	if logoGeneratorTool != nil {
		tools = append(tools, logoGeneratorTool.BaseTool)
	}

	adkAgent := baseAgent.NewAdkAgent(
		"图片收集助手",
		"帮助用户收集和搜索各类图片资源，包括内容图片、插画、架构图和Logo",
		prompt.GetImageCollectionPrompt(),
		tools,
	)

	return &ImageCollectionAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

func (a *ImageCollectionAgent) CollectImages(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	result, err := a.Generate(ctx, messages, a.adkAgent)
	if err != nil {
		return nil, err
	}

	return result, nil
}
