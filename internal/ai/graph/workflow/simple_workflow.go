package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
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
	graph := compose.NewGraph[*state.GraphState, *state.GraphState](compose.WithGenLocalState(func(context.Context) *struct{} { return &struct{}{} }))

	// 添加节点
	graph.AddLambdaNode("router", w.routerNode,
		compose.WithNodeName("智能路由节点"), safePointHandler("prompt_enhancer"))

	graph.AddLambdaNode("prompt_enhancer", w.promptEnhancerNode,
		compose.WithNodeName("提示词增强节点"), safePointHandler("code_generator"))

	graph.AddLambdaNode("code_generator", w.codeGeneratorNode,
		compose.WithNodeName("代码生成节点"), safePointHandler("quality_check"))

	graph.AddLambdaNode("quality_check", w.qualityCheckNode,
		compose.WithNodeName("代码质量检查节点"), safePointHandler(""))

	// 添加边
	entry := "router"
	if hooks := state.GetExecutionHooks(ctx); hooks != nil && hooks.NextNode != "" {
		entry = hooks.NextNode
	}
	switch entry {
	case "router", "prompt_enhancer", "code_generator", "quality_check", compose.END:
	default:
		return nil, fmt.Errorf("invalid checkpoint entry node: %s", entry)
	}
	graph.AddBranch(compose.START, compose.NewGraphBranch(func(context.Context, *state.GraphState) (string, error) { return entry, nil }, map[string]bool{"router": true, "prompt_enhancer": true, "code_generator": true, "quality_check": true, compose.END: true}))
	graph.AddEdge("router", "prompt_enhancer")
	graph.AddEdge("prompt_enhancer", "code_generator")
	graph.AddEdge("code_generator", "quality_check")

	// 质量检查分支：通过则结束，不通过则回到 code_generator
	graph.AddBranch("quality_check", compose.NewGraphBranch(
		func(ctx context.Context, input *state.GraphState) (string, error) {
			if input == nil || input.WorkFlowContext == nil || input.WorkFlowContext.NextNode == "" {
				return "", fmt.Errorf("quality node did not produce a next-node decision")
			}
			return input.WorkFlowContext.NextNode, nil
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

func appendQualityIssues(description string, qualityResult aimodel.QualityResult) string {
	var builder strings.Builder
	builder.WriteString(description)
	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}
	builder.WriteString("## Code quality issues\n")
	for _, issue := range qualityResult.Errors {
		builder.WriteString("- ")
		builder.WriteString(issue)
		builder.WriteString("\n")
	}
	if len(qualityResult.Suggestions) > 0 {
		builder.WriteString("\n## Suggestions\n")
		for _, suggestion := range qualityResult.Suggestions {
			builder.WriteString("- ")
			builder.WriteString(suggestion)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

// Reliable post-handlers propagate persistence errors; UI callbacks cannot do so.
func safePointHandler(next string) compose.GraphAddNodeOpt {
	return compose.WithStatePostHandler(func(ctx context.Context, out *state.GraphState, _ *struct{}) (*state.GraphState, error) {
		wc := out.WorkFlowContext
		if next == "" {
			nextNode := compose.END
			if !wc.QualityResult.IsValid {
				if wc.RetryCount < wc.MaxRetries {
					if budget := execution.GetBudget(ctx); budget != nil {
						if err := budget.ConsumeRetry(); err != nil {
							return nil, err
						}
					}
					wc.RetryCount++
					nextNode = "code_generator"
				} else {
					wc.Description = appendQualityIssues(wc.Description, wc.QualityResult)
				}
			}
			wc.NextNode = nextNode
		} else {
			wc.NextNode = next
		}
		for i := range wc.Plan.Todos {
			if wc.Plan.Todos[i].ID == "generate" && next == "quality_check" {
				wc.Plan.Todos[i].Status = "completed"
			}
			if wc.Plan.Todos[i].ID == "quality" && next == "" && wc.QualityResult.IsValid {
				wc.Plan.Todos[i].Status = "completed"
			}
			if wc.Plan.Todos[i].ID == "generate" && next == "" && wc.NextNode == "code_generator" {
				wc.Plan.Todos[i].Status = "pending"
			}
		}
		if hooks := state.GetExecutionHooks(ctx); hooks != nil && hooks.SafePoint != nil {
			if err := hooks.SafePoint(ctx, wc.NextNode, wc); err != nil {
				return nil, err
			}
		}
		return out, nil
	})
}

func (w *SimpleWorkflow) Execute(ctx context.Context, originalPrompt string) (*state.WorkFlowContext, error) {
	return w.ExecuteWithContext(ctx, &state.WorkFlowContext{
		OriginalPrompt: originalPrompt,
		CurrentStep:    "初始化",
		MaxRetries:     3,
	})
}

func (w *SimpleWorkflow) ExecuteWithContext(ctx context.Context, workflowContext *state.WorkFlowContext) (*state.WorkFlowContext, error) {
	runnable, err := w.CreateWorkflow(ctx)
	if err != nil {
		return nil, err
	}

	if workflowContext == nil {
		workflowContext = &state.WorkFlowContext{}
	}
	// NextNode is stored on the checkpoint envelope, not the JSON snapshot.
	// A compact inside the resumed node must checkpoint that same node again.
	if hooks := state.GetExecutionHooks(ctx); hooks != nil && hooks.NextNode != "" {
		workflowContext.NextNode = hooks.NextNode
	}
	if workflowContext.CurrentStep == "" {
		workflowContext.CurrentStep = "初始化"
	}
	if workflowContext.MaxRetries <= 0 {
		workflowContext.MaxRetries = 3
	}

	logger.Infof("初始输入: %s", workflowContext.OriginalPrompt)
	logger.Info("开始执行简化工作流")

	input := &state.GraphState{
		WorkFlowContext: workflowContext,
	}

	result, err := runnable.Invoke(state.BindWorkingContext(ctx, workflowContext), input)
	if err != nil {
		return nil, fmt.Errorf("执行工作流失败: %w", err)
	}

	workflowResult := result.WorkFlowContext
	logger.Infof(
		"工作流完成: appID=%d, step=%s, generationType=%s, files=%d, retry=%d/%d, qualityValid=%t, qualityErrors=%d, descriptionLength=%d",
		workflowResult.AppID,
		workflowResult.CurrentStep,
		workflowResult.GenerationType,
		len(workflowResult.CodeContent),
		workflowResult.RetryCount,
		workflowResult.MaxRetries,
		workflowResult.QualityResult.IsValid,
		len(workflowResult.QualityResult.Errors),
		len(workflowResult.Description),
	)
	logger.Info("简化工作流执行完成！")

	return result.WorkFlowContext, nil
}

func (w *SimpleWorkflow) ExecuteStream(ctx context.Context, workflowContext *state.WorkFlowContext) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	reader.SetAutomaticClose()
	stop := context.AfterFunc(ctx, func() { reader.Close() })
	workflowContext.StepCallback = func(stepNumber int, currentStep string) {
		eventType := workflowContext.StepEventType
		if eventType == "" {
			eventType = "step_completed"
		}
		nextStep := nextWorkflowStep(workflowContext, currentStep)
		data, err := json.Marshal(map[string]any{
			"type":        eventType,
			"stepNumber":  stepNumber,
			"currentStep": currentStep,
			"nextStep":    nextStep,
		})
		if err == nil {
			_ = writer.Send(&schema.Message{Content: string(data)}, nil)
		}
	}
	workflowContext.StreamChunkCallback = func(chunk string) {
		if chunk != "" {
			_ = writer.Send(&schema.Message{Content: chunk}, nil)
		}
	}
	go func() {
		defer stop()
		defer writer.Close()
		_, err := w.ExecuteWithContext(ctx, workflowContext)
		if err != nil {
			_ = writer.Send(nil, err)
		}
	}()
	return reader, nil
}

func nextWorkflowStep(workflowContext *state.WorkFlowContext, currentStep string) string {
	switch {
	case strings.Contains(currentStep, "智能路由"):
		return "提示词增强"
	case strings.Contains(currentStep, "提示词增强"):
		return "代码生成"
	case strings.Contains(currentStep, "代码生成"):
		return "代码质量检查"
	case strings.Contains(currentStep, "代码质量检查"):
		if workflowContext != nil && !workflowContext.QualityResult.IsValid {
			return "代码生成"
		}
		return "生成完成"
	default:
		return "处理中"
	}
}
