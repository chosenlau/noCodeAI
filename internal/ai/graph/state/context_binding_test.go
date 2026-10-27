package state

import (
	"context"
	"errors"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/stretchr/testify/require"
)

func TestWorkingSummarySafePointRetainsCurrentNodeAndRoundTrips(t *testing.T) {
	w := &WorkFlowContext{NextNode: "code_generator", CodeContent: map[string]string{"index.html": "draft"}}
	var checkpoint []byte
	ctx := WithExecutionHooks(context.Background(), &ExecutionHooks{SafePoint: func(_ context.Context, next string, wc *WorkFlowContext) error {
		require.Equal(t, "code_generator", next, "mid-node compact must not advance the graph")
		var err error
		checkpoint, err = wc.Snapshot()
		return err
	}})
	ctx = BindWorkingContext(ctx, w)
	summary := contextmgr.Snapshot{Version: 1, Summary: contextmgr.Summary{CurrentGoal: "goal", NextSafeAction: "read"}}
	require.NoError(t, contextmgr.GetBinding(ctx).Commit(ctx, "code", summary))
	restored, err := Restore(checkpoint)
	require.NoError(t, err)
	require.Equal(t, summary, restored.WorkingSummaries["code"])
	require.Equal(t, "draft", restored.CodeContent["index.html"])
	require.Equal(t, "code_generator", w.NextNode)
}

func TestWorkingSummaryRollsBackOnSaveFailureButRetainsSuspendedCheckpoint(t *testing.T) {
	for _, suspended := range []bool{false, true} {
		w := &WorkFlowContext{NextNode: "quality_check"}
		failure := errors.New("disk failure")
		if suspended {
			failure = &task.Suspended{Status: task.StatusPaused}
		}
		ctx := WithExecutionHooks(context.Background(), &ExecutionHooks{SafePoint: func(context.Context, string, *WorkFlowContext) error { return failure }})
		ctx = BindWorkingContext(ctx, w)
		err := contextmgr.GetBinding(ctx).Commit(ctx, "quality", contextmgr.Snapshot{Version: 1})
		require.ErrorIs(t, err, failure)
		_, exists := w.WorkingSummaries["quality"]
		require.Equal(t, suspended, exists)
	}
}

func TestTooLargePausesOnlyRecoverableWorkflow(t *testing.T) {
	w := &WorkFlowContext{}
	ctx := BindWorkingContext(context.Background(), w)
	require.ErrorIs(t, contextmgr.GetBinding(ctx).Blocked(ctx), contextmgr.ErrTooLarge)
	saved := false
	ctx = WithExecutionHooks(context.Background(), &ExecutionHooks{SafePoint: func(context.Context, string, *WorkFlowContext) error { saved = true; return nil }})
	ctx = BindWorkingContext(ctx, w)
	var paused *task.Suspended
	require.ErrorAs(t, contextmgr.GetBinding(ctx).Blocked(ctx), &paused)
	require.True(t, saved)
	ctx = BindWorkingContext(WithExecutionHooks(context.Background(), &ExecutionHooks{}), w)
	require.ErrorIs(t, contextmgr.GetBinding(ctx).Blocked(ctx), contextmgr.ErrTooLarge)
	ctx = WithExecutionHooks(context.Background(), &ExecutionHooks{SafePoint: func(context.Context, string, *WorkFlowContext) error { return errors.New("checkpoint unavailable") }})
	ctx = BindWorkingContext(ctx, w)
	require.ErrorIs(t, contextmgr.GetBinding(ctx).Blocked(ctx), contextmgr.ErrCommitFailed)
}
