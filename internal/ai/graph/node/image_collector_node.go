package node

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ImageCollectorNode struct {
	imageCollectionAgent *agent.ImageCollectionAgent
}

func NewImageCollectorNode(imageCollectionAgent *agent.ImageCollectionAgent) *compose.Lambda {
	node := &ImageCollectorNode{imageCollectionAgent: imageCollectionAgent}
	return compose.InvokableLambda(node.execute)
}

func (n *ImageCollectorNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 图片收集")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	originalPrompt := workflowContext.OriginalPrompt

	// 构建 messages
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: originalPrompt,
		},
	}

	var imageListStr string

	result, err := n.imageCollectionAgent.CollectImages(ctx, messages)
	if err != nil {
		logger.Errorf("图片收集失败: %v", err)
		imageListStr = ""
	} else {
		imageListStr = result.Content
	}

	workflowContext.ImageListStr = imageListStr
	state.NotifyStepCompleted(workflowContext, "图片收集")

	return input, nil
}
