package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/cloudwego/eino/compose"
	"github.com/stretchr/testify/require"
)

func TestSafePointStopsAndResumesOriginalGraph(t *testing.T) {
	var calls []string
	node := func(name string) *compose.Lambda {
		return compose.InvokableLambda(func(ctx context.Context, s *state.GraphState) (*state.GraphState, error) {
			calls = append(calls, name)
			if name == "quality" {
				s.WorkFlowContext.QualityResult.IsValid = true
			}
			return s, nil
		})
	}
	w := NewSimpleWorkflow(node("router"), node("enhancer"), node("code"), node("quality"))
	ctx := state.WithExecutionHooks(context.Background(), &state.ExecutionHooks{SafePoint: func(ctx context.Context, next string, wc *state.WorkFlowContext) error {
		if next == "code_generator" {
			return &task.Suspended{Status: task.StatusAwaitingApproval}
		}
		return nil
	}})
	_, err := w.ExecuteWithContext(ctx, &state.WorkFlowContext{})
	var stopped *task.Suspended
	require.True(t, errors.As(err, &stopped), "%v", err)
	require.Equal(t, []string{"router", "enhancer"}, calls)
	calls = nil
	ctx = state.WithExecutionHooks(context.Background(), &state.ExecutionHooks{NextNode: "code_generator"})
	wc, err := w.ExecuteWithContext(ctx, &state.WorkFlowContext{})
	require.NoError(t, err)
	require.True(t, wc.QualityResult.IsValid)
	require.Equal(t, []string{"code", "quality"}, calls)
	calls = nil
	ctx = state.WithExecutionHooks(context.Background(), &state.ExecutionHooks{NextNode: compose.END})
	_, err = w.ExecuteWithContext(ctx, wc)
	require.NoError(t, err)
	require.Empty(t, calls)
}

func TestCompactInsideResumedNodeDoesNotCheckpointRouter(t *testing.T) {
	unexpected := compose.InvokableLambda(func(context.Context, *state.GraphState) (*state.GraphState, error) {
		t.Fatal("resume must enter code_generator directly")
		return nil, nil
	})
	code := compose.InvokableLambda(func(ctx context.Context, s *state.GraphState) (*state.GraphState, error) {
		err := contextmgr.GetBinding(ctx).Commit(ctx, "code", contextmgr.Snapshot{Version: 1})
		return s, err
	})
	w := NewSimpleWorkflow(unexpected, unexpected, code, unexpected)
	saved := false
	ctx := state.WithExecutionHooks(context.Background(), &state.ExecutionHooks{NextNode: "code_generator", SafePoint: func(_ context.Context, next string, wc *state.WorkFlowContext) error {
		saved = true
		require.Equal(t, "code_generator", next)
		require.Equal(t, 1, wc.WorkingSummaries["code"].Version)
		return &task.Suspended{Status: task.StatusPaused}
	}})
	_, err := w.ExecuteWithContext(ctx, &state.WorkFlowContext{})
	var paused *task.Suspended
	require.ErrorAs(t, err, &paused)
	require.True(t, saved)
}
