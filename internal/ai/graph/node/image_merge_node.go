package node

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
)

type ImageMergeNode struct{}

func NewImageMergeNode() *compose.Lambda {
	node := &ImageMergeNode{}
	return compose.InvokableLambda(node.execute)
}

func (n *ImageMergeNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 图片合并")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	allImages := make([]aimodel.ImageSource, 0)
	allImages = append(allImages, workflowContext.ContentImage...)
	allImages = append(allImages, workflowContext.Illustrations...)
	allImages = append(allImages, workflowContext.Diagrams...)
	allImages = append(allImages, workflowContext.Logos...)

	logger.Infof("图片合并完成，共合并 %d 张图片", len(allImages))

	workflowContext.ImageList = allImages
	state.NotifyStepCompleted(workflowContext, "图片收集")

	return input, nil
}
