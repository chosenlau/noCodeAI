package taskstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Only an explicitly enabled test creates an isolated disposable database.
// It never truncates or cleans tables in the configured application database.
func mysqlStore(t *testing.T) *GormStore {
	t.Helper()
	if os.Getenv("RUN_TASK_MYSQL") != "1" {
		t.Skip("set RUN_TASK_MYSQL=1 for isolated MySQL transaction/runtime checks")
	}
	root, err := config.GetProjectRootPath()
	require.NoError(t, err)
	v := viper.New()
	v.SetConfigFile(filepath.Join(root, "config", "config-local.yml"))
	require.NoError(t, v.ReadInConfig())
	var cfg config.Config
	require.NoError(t, v.Unmarshal(&cfg))
	admin, err := gorm.Open(mysql.Open(cfg.GetDatabaseDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	name := fmt.Sprintf("nocode_task_test_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE DATABASE `"+name+"`").Error)
	t.Cleanup(func() {
		require.NoError(t, admin.Exec("DROP DATABASE `"+name+"`").Error)
		connection, _ := admin.DB()
		_ = connection.Close()
	})
	cfg.Database.Database = name
	db, err := gorm.Open(mysql.Open(cfg.GetDatabaseDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() { connection, _ := db.DB(); _ = connection.Close() })
	paths, err := filepath.Glob(filepath.Join(root, "migrations", "*.up.sql"))
	require.NoError(t, err)
	for _, path := range paths {
		schema, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, statement := range strings.Split(string(schema), ";") {
			if strings.TrimSpace(statement) != "" {
				require.NoError(t, db.Exec(statement).Error)
			}
		}
	}
	return NewGormStore(db)
}

func request(app int64, id string) task.StartRequest {
	return task.StartRequest{Kind: task.KindGraph, AppID: app, UserID: 1, RequestID: id, Input: json.RawMessage(`{"message":"hello"}`)}
}

func TestMySQLCreateAtomicIdempotentAndExclusive(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	// Simulate failure after the run/event inserts, before the outbox insert.
	require.NoError(t, s.db.Callback().Create().Before("gorm:create").Register("test:fail_outbox", func(tx *gorm.DB) {
		if tx.Statement.Table == "task_outbox" {
			tx.AddError(fmt.Errorf("injected outbox failure"))
		}
	}))
	_, _, err := s.CreateRun(ctx, request(1, "rollback"))
	require.Error(t, err)
	var count int64
	require.NoError(t, s.db.Model(&agentRunRow{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, s.db.Callback().Create().Remove("test:fail_outbox"))
	var wg sync.WaitGroup
	results := make(chan task.Run, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run, _, e := s.CreateRun(ctx, request(1, "same"))
			results <- run
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		require.NoError(t, e)
	}
	var first task.Run
	for run := range results {
		if first.ID == 0 {
			first = run
		}
		require.Equal(t, first.ID, run.ID)
	}
	busy, _, err := s.CreateRun(ctx, request(1, "other"))
	require.ErrorIs(t, err, task.ErrActiveRun)
	require.Equal(t, first.ID, busy.ID)
	changed := request(1, "same")
	changed.UserID = 2
	leak, _, err := s.CreateRun(ctx, changed)
	require.ErrorIs(t, err, task.ErrRequestConflict)
	require.Zero(t, leak.ID)
	changed = request(1, "same")
	changed.Input = json.RawMessage(`{"message":"changed"}`)
	_, _, err = s.CreateRun(ctx, changed)
	require.ErrorIs(t, err, task.ErrRequestConflict)
	require.NoError(t, s.db.Model(&runEventRow{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, s.db.Model(&outboxRow{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMySQLDetachedRuntimeReplayCapacityAndPanic(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	m := task.NewManager(s, task.Options{Enabled: true, WorkerCount: 1, PollInterval: 10 * time.Millisecond})
	started := make(chan int64, 8)
	release := make(chan struct{})
	var calls atomic.Int32
	m.Register(task.KindGraph, func(ctx context.Context, run task.Run, emit task.EmitFunc) (task.Result, error) {
		calls.Add(1)
		started <- run.ID
		if run.AppID == 3 {
			panic("simulated agent panic")
		}
		if run.AppID == 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return task.Result{}, ctx.Err()
			}
		}
		if err := emit(task.EventStepCompleted, map[string]string{"currentStep": "code_generator"}); err != nil {
			return task.Result{}, err
		}
		return task.Result{Files: map[string]string{"index.html": "immutable"}, GenerationType: "html", Description: "ready"}, nil
	})
	require.NoError(t, m.StartWorkers())
	t.Cleanup(func() {
		shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		require.NoError(t, m.Shutdown(shutdown))
	})
	// A second server cannot interrupt an existing runtime's runs.
	_, err := NewGormStore(s.db).AcquireRuntime(ctx)
	require.Error(t, err)
	requestCtx, cancel := context.WithCancel(ctx)
	first, _, err := m.Start(requestCtx, request(1, "detached"))
	require.NoError(t, err)
	cancel()
	select {
	case id := <-started:
		require.Equal(t, first.ID, id)
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	watchCtx, stopWatching := context.WithCancel(ctx)
	sub, err := m.Subscribe(watchCtx, first.ID, 1, 0)
	require.NoError(t, err)
	_, err = sub.Next(watchCtx)
	require.NoError(t, err)
	stopWatching()
	sub.Close()
	second, _, err := m.Start(ctx, request(2, "queued"))
	require.NoError(t, err)
	busy, _, err := m.Start(ctx, request(2, "queued-again"))
	require.ErrorIs(t, err, task.ErrActiveRun)
	require.Equal(t, second.ID, busy.ID)
	time.Sleep(80 * time.Millisecond)
	queued, err := m.Get(ctx, second.ID, 1)
	require.NoError(t, err)
	require.Equal(t, task.StatusPending, queued.Status)
	close(release)
	for _, id := range []int64{first.ID, second.ID} {
		require.Eventually(t, func() bool { run, e := m.Get(ctx, id, 1); return e == nil && run.Status == task.StatusCompleted }, 10*time.Second, 20*time.Millisecond)
	}
	// Replay from zero after a page was closed, including immutable code files.
	sub, err = m.Subscribe(ctx, first.ID, 1, 0)
	require.NoError(t, err)
	defer sub.Close()
	var sequence int64
	sawFiles := false
	for {
		event, e := sub.Next(ctx)
		if e == io.EOF {
			break
		}
		require.NoError(t, e)
		require.Equal(t, sequence+1, event.Sequence)
		sequence = event.Sequence
		if event.Type == task.EventCodeCompleted {
			require.JSONEq(t, `{"index.html":"immutable"}`, string(event.Payload))
			sawFiles = true
		}
	}
	require.True(t, sawFiles)
	// Duplicate dispatch after a delivery-ack loss must not invoke the handler twice.
	require.NoError(t, s.db.Model(&outboxRow{}).Where("aggregate_id = ? AND event_type = ?", first.ID, outboxTaskReady).Updates(map[string]any{"status": "processing", "claimed_at": time.Now().Add(-2 * time.Minute)}).Error)
	panicked, _, err := m.Start(ctx, request(3, "panic"))
	require.NoError(t, err)
	require.Eventually(t, func() bool { run, e := m.Get(ctx, panicked.ID, 1); return e == nil && run.Status == task.StatusFailed }, 10*time.Second, 20*time.Millisecond)
	require.EqualValues(t, 3, calls.Load())
	_, err = m.Get(ctx, first.ID, 999)
	require.ErrorIs(t, err, task.ErrNotFound)
}

func TestMySQLRestartInterruptsWithoutReexecution(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	run, _, err := s.CreateRun(ctx, request(1, "old"))
	require.NoError(t, err)
	_, err = s.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	m := task.NewManager(s, task.Options{Enabled: true})
	require.NoError(t, m.StartWorkers())
	require.NoError(t, m.Shutdown(ctx))
	got, err := s.GetRun(ctx, run.ID, 1)
	require.NoError(t, err)
	require.Equal(t, task.StatusInterrupted, got.Status)
	next, created, err := s.CreateRun(ctx, request(1, "new"))
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, run.ID, next.ID)
}

func TestMySQLSnapshotAndCompletionAreAtomic(t *testing.T) {
	s := mysqlStore(t)
	ctx := context.Background()
	run, _, err := s.CreateRun(ctx, request(1, "snapshot"))
	require.NoError(t, err)
	run, err = s.MarkRunning(ctx, run.ID)
	require.NoError(t, err)
	require.NoError(t, s.db.Callback().Create().Before("gorm:create").Register("test:fail_event", func(tx *gorm.DB) {
		if tx.Statement.Table == "run_event" {
			tx.AddError(fmt.Errorf("injected event failure"))
		}
	}))
	_, err = s.Complete(ctx, run.ID, task.Result{Files: map[string]string{"index.html": "version1"}})
	require.Error(t, err)
	var count int64
	require.NoError(t, s.db.Model(&sourceVersionRow{}).Count(&count).Error)
	require.Zero(t, count)
	unchanged, err := s.GetRunInternal(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, run.LastSequence, unchanged.LastSequence)
	require.Equal(t, task.StatusRunning, unchanged.Status)
	require.NoError(t, s.db.Callback().Create().Remove("test:fail_event"))
	finished, err := s.Complete(ctx, run.ID, task.Result{Files: map[string]string{"index.html": "version1"}})
	require.NoError(t, err)
	_, err = s.AppendEvent(ctx, run.ID, task.EventStepStarted, "late")
	require.Error(t, err)
	_, err = s.Complete(ctx, run.ID, task.Result{Files: map[string]string{"index.html": "version2"}})
	require.Error(t, err)
	files, err := s.GetSourceFiles(ctx, *finished.SourceVersionID)
	require.NoError(t, err)
	require.Equal(t, "version1", files["index.html"])
}
