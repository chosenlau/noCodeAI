package graph

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// CodeGenWorkflow is the legacy workflow with image collection support.
type CodeGenWorkflow struct {
	imageCollectorNode *compose.Lambda
	promptEnhancerNode *compose.Lambda
	routerNode         *compose.Lambda
	codeGeneratorNode  *compose.Lambda
	qualityCheckNode   *compose.Lambda
	projectBuilderNode *compose.Lambda
}

func NewCodeGenWorkflow(
	imageCollectorNode *compose.Lambda,
	promptEnhancerNode *compose.Lambda,
	routerNode *compose.Lambda,
	codeGeneratorNode *compose.Lambda,
	qualityCheckNode *compose.Lambda,
	projectBuilderNode *compose.Lambda,
) *CodeGenWorkflow {
	return &CodeGenWorkflow{
		imageCollectorNode: imageCollectorNode,
		promptEnhancerNode: promptEnhancerNode,
		routerNode:         routerNode,
		codeGeneratorNode:  codeGeneratorNode,
		qualityCheckNode:   qualityCheckNode,
		projectBuilderNode: projectBuilderNode,
	}
}

func (w *CodeGenWorkflow) CreateWorkflow(ctx context.Context) (compose.Runnable[*state.GraphState, *state.GraphState], error) {
	graph := compose.NewGraph[*state.GraphState, *state.GraphState]()
	graph.AddLambdaNode("image_collector", w.imageCollectorNode, compose.WithNodeName("Image collection"))
	graph.AddLambdaNode("prompt_enhancer", w.promptEnhancerNode, compose.WithNodeName("Prompt enhancement"))
	graph.AddLambdaNode("router", w.routerNode, compose.WithNodeName("Code generation routing"))
	graph.AddLambdaNode("code_generator", w.codeGeneratorNode, compose.WithNodeName("Code generation"))
	graph.AddLambdaNode("quality_check", w.qualityCheckNode, compose.WithNodeName("Code quality check"))

	graph.AddEdge(compose.START, "image_collector")
	graph.AddEdge("image_collector", "prompt_enhancer")
	graph.AddEdge("prompt_enhancer", "router")
	graph.AddEdge("router", "code_generator")
	graph.AddEdge("code_generator", "quality_check")

	branches := map[string]bool{"code_generator": true, compose.END: true}
	if w.projectBuilderNode != nil {
		graph.AddLambdaNode("project_builder", w.projectBuilderNode, compose.WithNodeName("Project build"))
		graph.AddEdge("project_builder", compose.END)
		branches["project_builder"] = true
	}

	graph.AddBranch("quality_check", compose.NewGraphBranch(
		func(ctx context.Context, input *state.GraphState) (string, error) {
			if input == nil || input.WorkFlowContext == nil {
				return compose.END, nil
			}
			workflowContext := input.WorkFlowContext
			if !workflowContext.QualityResult.IsValid {
				if workflowContext.MaxRetries <= 0 {
					workflowContext.MaxRetries = 3
				}
				if workflowContext.RetryCount < workflowContext.MaxRetries {
					workflowContext.RetryCount++
					return "code_generator", nil
				}
				return compose.END, nil
			}
			if workflowContext.GenerationType == enum.VueCodeGen && w.projectBuilderNode != nil {
				return "project_builder", nil
			}
			return compose.END, nil
		},
		branches,
	))

	runnable, err := graph.Compile(ctx, compose.WithGraphName("Code generation workflow with image search"))
	if err != nil {
		return nil, fmt.Errorf("compile code generation workflow: %w", err)
	}
	return runnable, nil
}

func (w *CodeGenWorkflow) ExecuteWithContext(ctx context.Context, workflowContext *state.WorkFlowContext) (*state.WorkFlowContext, error) {
	if workflowContext == nil {
		workflowContext = &state.WorkFlowContext{}
	}
	if workflowContext.MaxRetries <= 0 {
		workflowContext.MaxRetries = 3
	}
	runnable, err := w.CreateWorkflow(ctx)
	if err != nil {
		return nil, err
	}
	result, err := runnable.Invoke(ctx, &state.GraphState{WorkFlowContext: workflowContext})
	if err != nil {
		return nil, fmt.Errorf("execute code generation workflow: %w", err)
	}
	return result.WorkFlowContext, nil
}

func (w *CodeGenWorkflow) ExecuteStream(ctx context.Context, workflowContext *state.WorkFlowContext) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	if workflowContext == nil {
		workflowContext = &state.WorkFlowContext{}
	}
	workflowContext.StepCallback = func(stepNumber int, currentStep string) {
		data, err := json.Marshal(map[string]any{
			"type": "step_completed", "stepNumber": stepNumber,
			"currentStep": currentStep, "nextStep": "processing",
		})
		if err == nil {
			_ = writer.Send(&schema.Message{Content: string(data)}, nil)
		}
	}
	go func() {
		defer writer.Close()
		_, err := w.ExecuteWithContext(ctx, workflowContext)
		if err != nil {
			_ = writer.Send(nil, err)
		}
	}()
	return reader, nil
}

func ExecuteWorkflow(ctx context.Context, originalPrompt string) (*state.WorkFlowContext, error) {
	return nil, fmt.Errorf("legacy ExecuteWorkflow requires NewCodeGenWorkflow dependencies")
}

func ExecuteWorkflowStream(ctx context.Context, originalPrompt string) (*schema.StreamReader[string], *state.WorkFlowContext, error) {
	return nil, nil, fmt.Errorf("legacy ExecuteWorkflowStream requires NewCodeGenWorkflow dependencies")
}
