package taskstore

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/stretchr/testify/require"
)

func checkpointFixture(t *testing.T, s *GormStore, app int64) (task.Run, task.Checkpoint) {
	t.Helper()
	ctx := context.Background()
	run, _, err := s.CreateRun(ctx, request(app, "checkpoint-"+time.Now().Format("150405.000000000")))
	require.NoError(t, err)
	run, err = s.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	claim, err := NewExecutionStore(s.db).Claim(ctx, app, "attempt-1", 45*time.Second)
	require.NoError(t, err)
	cp := task.Checkpoint{Claim: claim, NextNode: "code_generator", State: json.RawMessage(`{"AppID":1}`), SnapshotPath: "immutable-snapshot", Compatibility: "test-v1", Plan: task.Plan{Goal: "generate site"}}
	cp.PlanHash = task.PlanHash(cp.Plan, 0)
	require.NoError(t, s.SaveCheckpoint(ctx, run.ID, cp))
	return run, cp
}

func TestMySQLCheckpointApprovalPauseResumeCancel(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	run, cp := checkpointFixture(t, s, 101)
	require.NoError(t, s.Suspend(ctx, run.ID, task.StatusAwaitingApproval))
	_, err := s.Control(ctx, run.ID, 2, "approve", cp.PlanHash)
	require.ErrorIs(t, err, task.ErrNotFound)
	_, err = s.Control(ctx, run.ID, 1, "approve", "stale")
	require.ErrorIs(t, err, task.ErrInvalidControl)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Control(ctx, run.ID, 1, "approve", cp.PlanHash); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, s.db.Model(&outboxRow{}).Where("event_id LIKE ?", "task-resume:%").Count(&count).Error)
	require.EqualValues(t, 1, count)
	_, err = s.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	approved, err := s.LoadCheckpoint(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, cp.PlanHash, approved.ApprovedHash)
	paused, err := s.Control(ctx, run.ID, 1, "pause", "")
	require.NoError(t, err)
	require.Equal(t, task.StatusPausing, paused.Status)
	_, err = s.Complete(ctx, run.ID, task.Result{})
	var stopped *task.Suspended
	require.ErrorAs(t, err, &stopped)
	require.NoError(t, s.Suspend(ctx, run.ID, task.StatusPaused))
	_, _, err = s.CreateRun(ctx, request(101, "new-while-paused"))
	require.ErrorIs(t, err, task.ErrActiveRun)
	resumed, err := s.Control(ctx, run.ID, 1, "resume", "")
	require.NoError(t, err)
	require.Equal(t, task.StatusPending, resumed.Status)
	_, err = s.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	canceled, err := s.Control(ctx, run.ID, 1, "cancel", "")
	require.NoError(t, err)
	require.Equal(t, task.StatusCancelling, canceled.Status)
	_, err = s.Complete(ctx, run.ID, task.Result{Files: map[string]string{"index.html": "draft"}})
	require.Error(t, err)
	_, err = s.Fail(ctx, run.ID, "canceled", false)
	require.NoError(t, err)
	final, err := s.GetRun(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, task.StatusCanceled, final.Status)
	require.NoError(t, s.db.Model(&sourceVersionRow{}).Where("run_id = ?", run.ID).Count(&count).Error)
	require.Zero(t, count)
	_, _, err = s.CreateRun(ctx, request(101, "new-after-cancel"))
	require.NoError(t, err)
}

func TestMySQLCheckpointRecoveryBudgetAndFencing(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	run, cp := checkpointFixture(t, s, 102)
	used := execution.BudgetSnapshot{ModelCalls: 3, Tokens: 123, ReservedTokens: 800}
	require.NoError(t, s.UpdateBudget(ctx, run.ID, cp.Claim, used, 3*time.Second))
	saved, err := s.LoadCheckpoint(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, used, saved.Budget)
	require.JSONEq(t, string(cp.State), string(saved.State))
	require.NoError(t, s.InterruptStale(ctx))
	recovered, err := s.GetRun(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, task.StatusRecoverable, recovered.Status)
	active, err := s.FindActiveRun(ctx, 102, 1)
	require.NoError(t, err)
	require.Equal(t, run.ID, active.ID)
	_, err = s.Control(ctx, run.ID, 1, "resume", "")
	require.NoError(t, err)
	_, err = s.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	// An older attempt must never overwrite the resumed run's checkpoint/budget.
	require.NoError(t, s.db.Model(&model.AppExecution{}).Where("app_id = ?", 102).Updates(map[string]any{"owner": "attempt-2", "epoch": cp.Claim.Epoch + 1}).Error)
	require.ErrorIs(t, s.SaveCheckpoint(ctx, run.ID, cp), execution.ErrFenced)
	require.ErrorIs(t, s.UpdateBudget(ctx, run.ID, cp.Claim, execution.BudgetSnapshot{}, 0), execution.ErrFenced)
}

func TestMySQLRestartFinishesCancellationWithoutFailureEvent(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	run, _ := checkpointFixture(t, s, 103)
	_, err := s.Control(ctx, run.ID, 1, "cancel", "")
	require.NoError(t, err)
	require.NoError(t, s.InterruptStale(ctx))
	final, err := s.GetRun(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, task.StatusCanceled, final.Status)
	require.Empty(t, final.ErrorMessage)
	events, err := s.ListEvents(ctx, run.ID, 0)
	require.NoError(t, err)
	var canceled bool
	for _, event := range events {
		require.NotEqual(t, EventError, event.Type)
		require.NotEqual(t, EventRunFailed, event.Type)
		canceled = canceled || event.Type == "run_canceled"
	}
	require.True(t, canceled)
}
