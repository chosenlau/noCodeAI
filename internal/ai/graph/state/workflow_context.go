package state

import (
	"sync"

	ai "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/schema"
)

type StepCallback func(stepNumber int, currentStep string)

type WorkFlowContext struct {
	AppID               int64
	mu                  sync.Mutex
	CurrentStep         string
	CodeContent         map[string]string
	OriginalPrompt      string
	ImageListStr        string
	ImageList           []ai.ImageSource
	EnhancedPrompt      string
	GenerationType      enum.CodeGenTypeEnum
	GenerateCodeDir     string
	BuildResultDir      string
	ErrorMessage        string
	QualityResult       ai.QualityResult
	ImageCollectionPlan ai.ImageCollectionPlan
	ContentImage        []ai.ImageSource
	Illustrations       []ai.ImageSource
	Diagrams            []ai.ImageSource
	Logos               []ai.ImageSource
	StepCallback        StepCallback
	StreamChunkCallback func(chunk string)
	StepEventType       string
	StepCounter         int
	RetryCount          int
	MaxRetries          int
	Description         string
}

type GraphState struct {
	WorkFlowContext *WorkFlowContext
	Content         []*schema.Message
}

func GetContext(graphState *GraphState) *WorkFlowContext {
	if graphState == nil {
		return nil
	}
	return graphState.WorkFlowContext
}

func NotifyStepStart(workflowCtx *WorkFlowContext, currentStep string) {
	if workflowCtx == nil {
		return
	}
	workflowCtx.mu.Lock()
	workflowCtx.CurrentStep = currentStep
	workflowCtx.StepEventType = "step_started"
	stepNumber := workflowCtx.StepCounter + 1
	callback := workflowCtx.StepCallback
	workflowCtx.mu.Unlock()

	if callback != nil {
		callback(stepNumber, currentStep)
	}
}

func NotifyStepCompleted(workflowCtx *WorkFlowContext, currentStep string) {
	if workflowCtx == nil {
		return
	}
	workflowCtx.mu.Lock()
	workflowCtx.StepCounter++
	workflowCtx.CurrentStep = currentStep
	workflowCtx.StepEventType = "step_completed"
	callback := workflowCtx.StepCallback
	workflowCtx.mu.Unlock()

	if callback != nil {
		callback(workflowCtx.StepCounter, currentStep)
	}
}
