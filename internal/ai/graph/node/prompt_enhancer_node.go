package node

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
)

type PromptEnhancerNode struct{}

func NewPromptEnhancerNode() *compose.Lambda {
	node := &PromptEnhancerNode{}
	return compose.InvokableLambda(node.execute)
}

func (n *PromptEnhancerNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 提示词增强")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	originalPrompt := workflowContext.OriginalPrompt
	imageListStr := workflowContext.ImageListStr
	imageList := workflowContext.ImageList

	var enhancedPrompt string
	if originalPrompt != "" {
		enhancedPromptBuilder := originalPrompt

		if len(imageList) > 0 || imageListStr != "" {
			enhancedPromptBuilder += "\n\n## 可用素材资源\n"
			enhancedPromptBuilder += "请在生成网站使用以下图片资源，将这些图片合理地嵌入到网站的相应位置中。\n"

			if len(imageList) > 0 {
				for _, image := range imageList {
					enhancedPromptBuilder += fmt.Sprintf("- %s：%s（%s）\n",
						image.Category.Text(),
						image.Description,
						image.Url)
				}
			} else {
				enhancedPromptBuilder += imageListStr
			}
		}

		enhancedPrompt = enhancedPromptBuilder
	}

	logger.Infof("提示词增强完成，增强后长度: %d 字符", len(enhancedPrompt))

	workflowContext.EnhancedPrompt = enhancedPrompt
	state.NotifyStepCompleted(workflowContext, "提示词增强")

	return input, nil
}
