package taskstore

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMySQLExecutionFencingAndJournal(t *testing.T) {
	runs := mysqlStore(t)
	s := NewExecutionStore(runs.db)
	ctx := context.Background()
	first, err := s.Claim(ctx, 101, "first", time.Minute)
	require.NoError(t, err)
	_, err = s.Claim(ctx, 101, "other", time.Minute)
	require.ErrorIs(t, err, execution.ErrBusy)
	require.NoError(t, s.Renew(ctx, first, time.Minute))
	require.NoError(t, runs.db.Model(&model.AppExecution{}).Where("app_id = ?", 101).Update("lease_until", gorm.Expr("TIMESTAMPADD(SECOND, -60, NOW(3))")).Error)
	require.ErrorIs(t, s.Renew(ctx, first, time.Minute), execution.ErrFenced)
	second, err := s.Claim(ctx, 101, "second", time.Minute)
	require.NoError(t, err)
	require.Greater(t, second.Epoch, first.Epoch)
	_, err = s.Publish(ctx, execution.Publication{Claim: first, Path: "old", VersionID: 11}, map[string]string{"a": "old"}, "html")
	require.ErrorIs(t, err, execution.ErrFenced)
	require.NoError(t, s.Release(ctx, first))
	require.NoError(t, s.Renew(ctx, second, time.Minute))
	_, err = s.Publish(ctx, execution.Publication{Claim: second, Path: "new", VersionID: 12}, map[string]string{"a": "new"}, "html")
	require.NoError(t, err)
	current, err := s.Current(ctx, 101)
	require.NoError(t, err)
	require.EqualValues(t, 12, current.ID)
	require.Equal(t, "new", current.Path)
	// The same base revision cannot be published a second time.
	_, err = s.Publish(ctx, execution.Publication{Claim: second, Path: "again", VersionID: 13}, nil, "html")
	require.ErrorIs(t, err, execution.ErrFenced)
	call := execution.ToolCall{ExecutionID: "second", CallID: "tool1", ToolName: "write", ArgumentsHash: "hash"}
	record, claimed, err := s.BeginTool(ctx, call)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Equal(t, "running", record.Status)
	_, claimed, err = s.BeginTool(ctx, call)
	require.NoError(t, err)
	require.False(t, claimed)
	require.NoError(t, s.FinishTool(ctx, call, execution.ToolRecord{Status: "succeeded", Result: "written"}))
	record, claimed, err = s.BeginTool(ctx, call)
	require.NoError(t, err)
	require.False(t, claimed)
	require.Equal(t, "written", record.Result)
	call.ArgumentsHash = "different"
	_, _, err = s.BeginTool(ctx, call)
	require.ErrorIs(t, err, execution.ErrToolConflict)
}

func TestMySQLRunPublicationRollback(t *testing.T) {
	runs := mysqlStore(t)
	s := NewExecutionStore(runs.db)
	ctx := context.Background()
	run, _, err := runs.CreateRun(ctx, request(102, "atomic-publication"))
	require.NoError(t, err)
	_, err = runs.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	claim, err := s.Claim(ctx, 102, "owner", time.Minute)
	require.NoError(t, err)
	require.NoError(t, runs.db.Callback().Create().Before("gorm:create").Register("test:publication_fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "source_version" {
			tx.AddError(fmt.Errorf("injected snapshot failure"))
		}
	}))
	result := Result{Publication: &execution.Publication{Claim: claim, Path: "frozen"}, Files: map[string]string{"index.html": "ok"}, GenerationType: "html"}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	result.Validate = canceled.Err
	_, err = runs.Complete(ctx, run.ID, result)
	require.ErrorIs(t, err, context.Canceled)
	current, err := s.Current(ctx, 102)
	require.NoError(t, err)
	require.Zero(t, current.ID)
	result.Validate = nil
	_, err = runs.Complete(ctx, run.ID, result)
	require.Error(t, err)
	current, err = s.Current(ctx, 102)
	require.NoError(t, err)
	require.Zero(t, current.ID)
	require.NoError(t, runs.db.Callback().Create().Remove("test:publication_fail"))
	completed, err := runs.Complete(ctx, run.ID, result)
	require.NoError(t, err)
	current, err = s.Current(ctx, 102)
	require.NoError(t, err)
	require.Equal(t, *completed.SourceVersionID, current.ID)
}

func TestMySQLConcurrentExecutionClaims(t *testing.T) {
	runs := mysqlStore(t)
	s := NewExecutionStore(runs.db)
	var wg sync.WaitGroup
	results := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(owner int) {
			defer wg.Done()
			_, err := s.Claim(context.Background(), 103, fmt.Sprintf("owner-%d", owner), time.Minute)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, execution.ErrBusy)
		}
	}
	require.Equal(t, 1, winners)
}
