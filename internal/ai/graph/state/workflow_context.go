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
	StepCounter         int
	RetryCount          int
	MaxRetries          int
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

func NotifyStepCompleted(workflowCtx *WorkFlowContext, currentStep string) {
	if workflowCtx == nil {
		return
	}
	workflowCtx.mu.Lock()
	workflowCtx.StepCounter++
	workflowCtx.CurrentStep = currentStep
	callback := workflowCtx.StepCallback
	workflowCtx.mu.Unlock()

	if callback != nil {
		callback(workflowCtx.StepCounter, currentStep)
	}
}
