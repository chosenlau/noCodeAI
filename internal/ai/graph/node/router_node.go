package node

import (
	"context"
	"fmt"
	"time"

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

const routingTimeout = 90 * time.Second

func NewRouterNode(routingAgent *agent.CodeGenTypeRoutingAgent) *compose.Lambda {
	return compose.InvokableLambda((&RouterNode{routingAgent: routingAgent}).execute)
}

func (n *RouterNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 智能路由")
	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		workflowContext = &state.WorkFlowContext{}
		input.WorkFlowContext = workflowContext
	}
	state.NotifyStepStart(workflowContext, "智能路由")
	if workflowContext.GenerationType != "" {
		logger.Infof("已有代码生成类型，跳过 AI 路由: %s", workflowContext.GenerationType)
		state.NotifyStepCompleted(workflowContext, "智能路由")
		return input, nil
	}
	if workflowContext.OriginalPrompt == "" {
		return nil, fmt.Errorf("original prompt is empty")
	}
	if n.routingAgent == nil {
		return nil, fmt.Errorf("code generation routing agent is not initialized")
	}

	routingCtx, cancel := context.WithTimeout(ctx, routingTimeout)
	defer cancel()
	logger.Infof("starting AI code generation routing, timeout=%s", routingTimeout)
	generationType, err := n.routingAgent.RouteCodeGenType(
		routingCtx,
		buildRoutingMessages(workflowContext.OriginalPrompt),
	)
	if err != nil {
		if routingCtx.Err() != nil {
			return nil, fmt.Errorf("code generation routing timed out after %s: %w", routingTimeout, routingCtx.Err())
		}
		return nil, fmt.Errorf("code generation routing failed: %w", err)
	}
	if enum.CodeGenTypeTextMap[generationType] == "" {
		return nil, fmt.Errorf("unsupported code generation type: %q", generationType)
	}

	logger.Infof("AI智能路由完成，选择类型: %s (%s)", generationType, enum.CodeGenTypeTextMap[generationType])
	workflowContext.GenerationType = generationType
	state.NotifyStepCompleted(workflowContext, "智能路由")
	return input, nil
}

func buildRoutingMessages(originalPrompt string) []*schema.Message {
	return []*schema.Message{{Role: schema.User, Content: originalPrompt}}
}
