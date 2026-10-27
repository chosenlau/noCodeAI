package node

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core/task"
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
	state.NotifyStepStart(workflowContext, "提示词增强")

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
	if len(workflowContext.Plan.Todos) == 0 {
		goal := workflowContext.Plan.Goal
		if goal == "" {
			goal = originalPrompt
		}
		workflowContext.Plan = task.Plan{
			Goal:           goal,
			GenerationType: string(workflowContext.GenerationType),
			ExecutionMode:  workflowContext.ExecutionMode,
			Constraints:    []string{"遵循原始需求和项目现有技术栈", "仅在当前项目工作目录内修改文件"},
			Acceptance:     []string{"生成完整代码文件", "通过现有代码质量检查"},
			Todos:          []task.Todo{{ID: "generate", Title: "根据需求生成或修改代码", Status: "pending"}, {ID: "quality", Title: "检查代码并修复问题", Status: "pending"}},
		}
		if workflowContext.ExecutionMode == state.ModeModify {
			workflowContext.Plan.Constraints = append(workflowContext.Plan.Constraints, "先读取现有代码，只修改与本次需求相关的内容，不重建项目或更换技术栈")
			workflowContext.Plan.Acceptance = []string{"完成本次修改并保留原有功能", "通过现有代码质量检查"}
		}
	}
	state.NotifyStepCompleted(workflowContext, "提示词增强")

	return input, nil
}
