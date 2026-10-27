package state

import (
	"context"
	"encoding/json"

	ai "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/pkg/enum"
)

// Snapshot is deliberately separate from the runtime context. Add fields here
// when a new node needs durable input, and bump the compatibility version.
type Snapshot struct {
	AppID            int64
	OriginalPrompt   string
	EnhancedPrompt   string
	GenerationType   enum.CodeGenTypeEnum
	ExecutionMode    string
	CodeContent      map[string]string
	QualityResult    ai.QualityResult
	Description      string
	RetryCount       int
	MaxRetries       int
	StepCounter      int
	CurrentStep      string
	ImageListStr     string
	ImageList        []ai.ImageSource
	Plan             task.Plan
	WorkingSummaries map[string]contextmgr.Snapshot
}

func (w *WorkFlowContext) Snapshot() ([]byte, error) {
	return json.Marshal(Snapshot{
		AppID: w.AppID, OriginalPrompt: w.OriginalPrompt, EnhancedPrompt: w.EnhancedPrompt,
		GenerationType: w.GenerationType, ExecutionMode: w.ExecutionMode, CodeContent: w.CodeContent, QualityResult: w.QualityResult,
		Description: w.Description, RetryCount: w.RetryCount, MaxRetries: w.MaxRetries,
		StepCounter: w.StepCounter, CurrentStep: w.CurrentStep,
		ImageListStr: w.ImageListStr, ImageList: w.ImageList, Plan: w.Plan, WorkingSummaries: w.WorkingSummaries,
	})
}
func Restore(data []byte) (*WorkFlowContext, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &WorkFlowContext{AppID: s.AppID, OriginalPrompt: s.OriginalPrompt, EnhancedPrompt: s.EnhancedPrompt, GenerationType: s.GenerationType, ExecutionMode: s.ExecutionMode, CodeContent: s.CodeContent, QualityResult: s.QualityResult, Description: s.Description, RetryCount: s.RetryCount, MaxRetries: s.MaxRetries, StepCounter: s.StepCounter, CurrentStep: s.CurrentStep, ImageListStr: s.ImageListStr, ImageList: s.ImageList, Plan: s.Plan, WorkingSummaries: s.WorkingSummaries}, nil
}

type executionHooksKey struct{}
type ExecutionHooks struct {
	NextNode  string
	SafePoint func(context.Context, string, *WorkFlowContext) error
}

func WithExecutionHooks(ctx context.Context, h *ExecutionHooks) context.Context {
	return context.WithValue(ctx, executionHooksKey{}, h)
}
func GetExecutionHooks(ctx context.Context) *ExecutionHooks {
	h, _ := ctx.Value(executionHooksKey{}).(*ExecutionHooks)
	return h
}
