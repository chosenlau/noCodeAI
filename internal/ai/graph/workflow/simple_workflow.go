package workflow

import (
	"context"
	"fmt"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/compose"
)

type SimpleWorkflow struct {
	routerNode         *compose.Lambda
	promptEnhancerNode *compose.Lambda
	codeGeneratorNode  *compose.Lambda
	qualityCheckNode   *compose.Lambda
}

func NewSimpleWorkflow(
	routerNode *compose.Lambda,
	promptEnhancerNode *compose.Lambda,
	codeGeneratorNode *compose.Lambda,
	qualityCheckNode *compose.Lambda,
) *SimpleWorkflow {
	return &SimpleWorkflow{
		routerNode:         routerNode,
		promptEnhancerNode: promptEnhancerNode,
		codeGeneratorNode:  codeGeneratorNode,
		qualityCheckNode:   qualityCheckNode,
	}
}

func (w *SimpleWorkflow) CreateWorkflow(ctx context.Context) (compose.Runnable[*state.GraphState, *state.GraphState], error) {
	graph := compose.NewGraph[*state.GraphState, *state.GraphState]()

	// 添加节点
	graph.AddLambdaNode("router", w.routerNode,
		compose.WithNodeName("智能路由节点"))

	graph.AddLambdaNode("prompt_enhancer", w.promptEnhancerNode,
		compose.WithNodeName("提示词增强节点"))

	graph.AddLambdaNode("code_generator", w.codeGeneratorNode,
		compose.WithNodeName("代码生成节点"))

	graph.AddLambdaNode("quality_check", w.qualityCheckNode,
		compose.WithNodeName("代码质量检查节点"))

	// 添加边
	graph.AddEdge(compose.START, "router")
	graph.AddEdge("router", "prompt_enhancer")
	graph.AddEdge("prompt_enhancer", "code_generator")
	graph.AddEdge("code_generator", "quality_check")

	// 质量检查分支：通过则结束，不通过则回到 code_generator
	graph.AddBranch("quality_check", compose.NewGraphBranch(
		func(ctx context.Context, input *state.GraphState) (string, error) {
			workflowContext := input.WorkFlowContext
			if workflowContext == nil {
				logger.Warn("质检分支: 无上下文，默认结束")
				return compose.END, nil
			}

			qualityResult := workflowContext.QualityResult
			if !qualityResult.IsValid {
				maxRetries := workflowContext.MaxRetries
				if maxRetries <= 0 {
					maxRetries = 3
				}
				if workflowContext.RetryCount >= maxRetries {
					return compose.END, fmt.Errorf("quality check failed after %d retries", maxRetries)
				}
				workflowContext.RetryCount++
				logger.Errorf("代码质检失败，需要重新生成代码: %v", qualityResult.Errors)
				return "code_generator", nil
			}

			logger.Info("代码质检通过，工作流结束")
			return compose.END, nil
		},
		map[string]bool{
			"code_generator": true,
			compose.END:      true,
		},
	))

	runnable, err := graph.Compile(ctx, compose.WithGraphName("简化代码生成工作流"))
	if err != nil {
		return nil, fmt.Errorf("编译工作流失败: %w", err)
	}

	return runnable, nil
}

func (w *SimpleWorkflow) Execute(ctx context.Context, originalPrompt string) (*state.WorkFlowContext, error) {
	runnable, err := w.CreateWorkflow(ctx)
	if err != nil {
		return nil, err
	}

	initialContext := &state.WorkFlowContext{
		OriginalPrompt: originalPrompt,
		CurrentStep:    "初始化",
		GenerationType: enum.HtmlCodeGen, // 默认类型
		MaxRetries:     3,
	}

	logger.Infof("初始输入: %s", initialContext.OriginalPrompt)
	logger.Info("开始执行简化工作流")

	input := &state.GraphState{
		WorkFlowContext: initialContext,
	}

	result, err := runnable.Invoke(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("执行工作流失败: %w", err)
	}

	logger.Infof("最终结果: %v", result.WorkFlowContext)
	logger.Info("简化工作流执行完成！")

	return result.WorkFlowContext, nil
}

// RunSimpleWorkflow 保留旧的测试函数（用于演示）
func RunSimpleWorkflow() error {
	ctx := context.Background()

	// 创建测试节点
	routerNode := node.NewRouterNode(nil) // 需要注入真实 Agent
	promptEnhancerNode := node.NewPromptEnhancerNode()
	codeGeneratorNode := node.NewCodeGeneratorNode(nil)   // 需要注入真实 Facade
	qualityCheckNode := node.NewCodeQualityCheckNode(nil) // 需要注入真实 Agent

	workflow := NewSimpleWorkflow(
		routerNode,
		promptEnhancerNode,
		codeGeneratorNode,
		qualityCheckNode,
	)

	_, err := workflow.Execute(ctx, "创建一个简单的个人主页")
	return err
}
