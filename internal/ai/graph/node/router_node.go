package node

import (
	"context"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type RouterNode struct {
	routingAgent *agent.CodeGenTypeRoutingAgent
}

func NewRouterNode(routingAgent *agent.CodeGenTypeRoutingAgent) *compose.Lambda {
	node := &RouterNode{
		routingAgent: routingAgent,
	}
	return compose.InvokableLambda(node.execute)
}

func (n *RouterNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 智能路由")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		workflowContext = &state.WorkFlowContext{}
	}

	originalPrompt := workflowContext.OriginalPrompt
	if originalPrompt == "" {
		logger.Warn("原始提示词为空，使用默认HTML类型")
		input.WorkFlowContext.GenerationType = enum.HtmlCodeGen
		return input, nil
	}

	// 构建 messages
	messages := buildRoutingMessages(originalPrompt)
	// 调用 agent
	generationType, err := n.routingAgent.RouteCodeGenType(ctx, messages)
	if err != nil {
		logger.Errorf("AI智能路由失败，使用默认HTML类型: %v", err)
		generationType = enum.HtmlCodeGen
	} else {
		logger.Infof("AI智能路由完成，选择类型: %s (%s)", generationType, enum.CodeGenTypeTextMap[generationType])
	}

	logger.Infof("路由决策完成，选择类型: %s", enum.CodeGenTypeTextMap[generationType])
	input.WorkFlowContext.GenerationType = generationType
	state.NotifyStepCompleted(input.WorkFlowContext, "智能路由")
	return input, nil
}

func buildRoutingMessages(originalPrompt string) []*schema.Message {
	return []*schema.Message{
		{
			Role:    schema.User,
			Content: originalPrompt,
		},
	}
}
