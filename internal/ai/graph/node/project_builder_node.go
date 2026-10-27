package node

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core/builder"
	"github.com/cloudwego/eino/compose"
)

type ProjectBuilderNode struct{}

func NewProjectBuilderNode() *compose.Lambda {
	node := &ProjectBuilderNode{}
	return compose.InvokableLambda(node.execute)
}

func (n *ProjectBuilderNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 项目构建")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	generatedCodeDir := workflowContext.GenerateCodeDir

	result, err := builder.BuildProjectContext(ctx, generatedCodeDir)
	if err != nil {
		return nil, fmt.Errorf("项目构建失败: %w", err)
	}
	buildResultDir := result.Dist

	logger.Infof("项目构建节点完成，最终目录: %s", buildResultDir)

	workflowContext.BuildResultDir = buildResultDir
	state.NotifyStepCompleted(workflowContext, "项目构建")

	return input, nil
}
