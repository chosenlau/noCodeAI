package node

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ImagePlanNode struct {
	planAgent *agent.ImageCollectionPlanAgent
}

func NewImagePlanNode(planAgent *agent.ImageCollectionPlanAgent) *compose.Lambda {
	node := &ImagePlanNode{planAgent: planAgent}
	return compose.InvokableLambda(node.execute)
}

func (n *ImagePlanNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 图片计划生成")

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

	plan, err := n.planAgent.PlanImageCollection(ctx, messages)
	if err != nil {
		logger.Errorf("图片计划生成失败: %v", err)
		// 失败时使用空计划
		workflowContext.ImageCollectionPlan = aimodel.ImageCollectionPlan{}
	} else {
		workflowContext.ImageCollectionPlan = plan
		logger.Info("生成图片收集计划，准备启动并发分支")
	}

	state.NotifyStepCompleted(workflowContext, "图片计划")

	return input, nil
}
