package taskstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Domain contracts belong to core/task; SQL implementation stays in the DAL.
type Run = task.Run
type StartRequest = task.StartRequest
type Event = task.Event
type Result = task.Result
type outboxMessage = task.OutboxMessage

const (
	StatusPending            = task.StatusPending
	StatusRunning            = task.StatusRunning
	StatusCompleted          = task.StatusCompleted
	StatusFailed             = task.StatusFailed
	StatusInterrupted        = task.StatusInterrupted
	EventRunCreated          = task.EventRunCreated
	EventRunStarted          = task.EventRunStarted
	EventRunCompleted        = task.EventRunCompleted
	EventRunFailed           = task.EventRunFailed
	EventCodeCompleted       = task.EventCodeCompleted
	EventDescriptionComplete = task.EventDescriptionComplete
	EventError               = task.EventError
	EventDone                = task.EventDone
	outboxTaskReady          = task.OutboxTaskReady
	outboxEventAvailable     = task.OutboxEventAvailable
)

var (
	ErrNotFound        = task.ErrNotFound
	ErrActiveRun       = task.ErrActiveRun
	ErrRequestConflict = task.ErrRequestConflict
)

type agentRunRow = model.AgentRun
type runEventRow = model.RunEvent
type sourceVersionRow = model.SourceVersion
type outboxRow = model.TaskOutbox

type GormStore struct {
	db       *gorm.DB
	owner    *sql.Conn
	lockName string
}

func NewGormStore(db *gorm.DB) *GormStore { return &GormStore{db: db} }

// A dedicated MySQL connection owns the runtime lock for its whole lifetime.
// A second process must fail startup rather than interrupt the first process.
func (s *GormStore) AcquireRuntime(ctx context.Context) (func(), error) {
	db, err := s.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var name string
	if err = conn.QueryRowContext(ctx, "SELECT CONCAT('nocode-task:', DATABASE())").Scan(&name); err != nil {
		conn.Close()
		return nil, err
	}
	var acquired int
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&acquired); err != nil || acquired != 1 {
		conn.Close()
		return nil, fmt.Errorf("detached task runtime requires one owner per database (lock acquired=%d): %v", acquired, err)
	}
	s.owner, s.lockName = conn, name
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(releaseCtx, "SELECT RELEASE_LOCK(?)", name)
		_ = conn.Close()
	}, nil
}

func (s *GormStore) CheckRuntime(ctx context.Context) error {
	var owned bool
	if s.owner == nil {
		return fmt.Errorf("runtime owner not acquired")
	}
	if err := s.owner.QueryRowContext(ctx, "SELECT IS_USED_LOCK(?) = CONNECTION_ID()", s.lockName).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("runtime lock lost")
	}
	return nil
}

func replayRun(row agentRunRow, request StartRequest) (Run, bool, error) {
	if row.UserID != request.UserID || row.AppID != request.AppID || row.Kind != request.Kind || !bytes.Equal(row.InputJSON, request.Input) {
		// MySQL normalizes JSON spacing. Compare decoded values as compact JSON.
		var stored, incoming any
		_ = json.Unmarshal(row.InputJSON, &stored)
		_ = json.Unmarshal(request.Input, &incoming)
		a, _ := json.Marshal(stored)
		b, _ := json.Marshal(incoming)
		if row.UserID != request.UserID || row.AppID != request.AppID || row.Kind != request.Kind || !bytes.Equal(a, b) {
			return Run{}, false, ErrRequestConflict
		}
	}
	return rowToRun(row), false, nil
}

func (s *GormStore) CreateRun(ctx context.Context, request StartRequest) (Run, bool, error) {
	var existing agentRunRow
	err := s.db.WithContext(ctx).Where("request_id = ?", request.RequestID).First(&existing).Error
	if err == nil {
		return replayRun(existing, request)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Run{}, false, err
	}

	var active agentRunRow
	err = s.db.WithContext(ctx).
		Where("app_id = ? AND user_id = ? AND active_key IS NOT NULL", request.AppID, request.UserID).
		Order("created_at DESC").First(&active).Error
	if err == nil {
		if active.RequestID == request.RequestID {
			return replayRun(active, request)
		}
		return rowToRun(active), false, ErrActiveRun
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Run{}, false, err
	}

	runID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return Run{}, false, err
	}
	eventID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return Run{}, false, err
	}
	outboxID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return Run{}, false, err
	}
	now := time.Now()
	activeKey := strconv.FormatInt(request.AppID, 10)
	runRow := agentRunRow{
		ID:           runID,
		AppID:        request.AppID,
		UserID:       request.UserID,
		RequestID:    request.RequestID,
		Kind:         request.Kind,
		Status:       StatusPending,
		InputJSON:    request.Input,
		ActiveKey:    &activeKey,
		LastSequence: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	createdPayload, _ := json.Marshal(map[string]any{"runId": strconv.FormatInt(runID, 10), "status": StatusPending})
	eventRow := runEventRow{ID: eventID, RunID: runID, Sequence: 1, EventType: EventRunCreated, Payload: createdPayload, CreatedAt: now}
	outbox := outboxRow{
		ID: outboxID, EventID: fmt.Sprintf("task-ready:%d", runID), AggregateType: "agent_run",
		AggregateID: runID, EventType: outboxTaskReady, Payload: []byte(`{}`), Status: "pending",
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&runRow).Error; err != nil {
			return err
		}
		if err := tx.Create(&eventRow).Error; err != nil {
			return err
		}
		return tx.Create(&outbox).Error
	})
	if err != nil {
		if findErr := s.db.WithContext(ctx).Where("request_id = ?", request.RequestID).First(&existing).Error; findErr == nil {
			return replayRun(existing, request)
		}
		if findErr := s.db.WithContext(ctx).Where("active_key = ?", activeKey).First(&active).Error; findErr == nil {
			if active.RequestID == request.RequestID {
				return replayRun(active, request)
			}
			return rowToRun(active), false, ErrActiveRun
		}
		return Run{}, false, err
	}
	return rowToRun(runRow), true, nil
}

func (s *GormStore) GetRun(ctx context.Context, runID, userID int64) (Run, error) {
	var row agentRunRow
	err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", runID, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Run{}, ErrNotFound
	}
	return s.withCheckpoint(ctx, row, err)
}

func (s *GormStore) GetRunInternal(ctx context.Context, runID int64) (Run, error) {
	var row agentRunRow
	err := s.db.WithContext(ctx).Where("id = ?", runID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Run{}, ErrNotFound
	}
	return s.withCheckpoint(ctx, row, err)
}

func (s *GormStore) withCheckpoint(ctx context.Context, row agentRunRow, err error) (Run, error) {
	if err != nil {
		return Run{}, err
	}
	run := rowToRun(row)
	run.Checkpoint, err = s.LoadCheckpoint(ctx, row.ID)
	return run, err
}

func (s *GormStore) FindActiveRun(ctx context.Context, appID, userID int64) (Run, error) {
	var row agentRunRow
	err := s.db.WithContext(ctx).
		Where("app_id = ? AND user_id = ? AND active_key IS NOT NULL", appID, userID).
		Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Run{}, ErrNotFound
	}
	return s.withCheckpoint(ctx, row, err)
}

func (s *GormStore) ListEvents(ctx context.Context, runID, after int64) ([]Event, error) {
	var rows []runEventRow
	if err := s.db.WithContext(ctx).Where("run_id = ? AND sequence > ?", runID, after).Order("sequence ASC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, Event{RunID: row.RunID, Sequence: row.Sequence, Type: row.EventType, Payload: row.Payload, CreatedAt: row.CreatedAt})
	}
	return events, nil
}

func (s *GormStore) GetSourceFiles(ctx context.Context, versionID int64) (map[string]string, error) {
	var row sourceVersionRow
	if err := s.db.WithContext(ctx).Where("id = ?", versionID).First(&row).Error; err != nil {
		return nil, err
	}
	var files map[string]string
	if err := json.Unmarshal(row.FilesJSON, &files); err != nil {
		return nil, err
	}
	return files, nil
}

func (s *GormStore) MarkRunning(ctx context.Context, runID int64) (Run, error) {
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != StatusPending {
			return fmt.Errorf("cannot start run %d from status %s", runID, row.Status)
		}
		if err := tx.Model(&agentRunRow{}).Where("id = ?", runID).Updates(map[string]any{"status": StatusRunning, "started_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return s.appendEventTx(tx, &row, EventRunStarted, map[string]any{"status": StatusRunning})
	}); err != nil {
		return Run{}, err
	}
	return s.GetRunInternal(ctx, runID)
}

func (s *GormStore) AppendEvent(ctx context.Context, runID int64, eventType string, payload any) (Event, error) {
	var created Event
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != StatusRunning && row.Status != task.StatusPausing {
			return fmt.Errorf("cannot append event to run in status %s", row.Status)
		}
		if err := s.appendEventTx(tx, &row, eventType, payload); err != nil {
			return err
		}
		created = Event{RunID: runID, Sequence: row.LastSequence + 1, Type: eventType, CreatedAt: time.Now()}
		created.Payload, _ = json.Marshal(payload)
		return nil
	})
	return created, err
}

func (s *GormStore) Complete(ctx context.Context, runID int64, result Result) (Run, error) {
	filesJSON, err := json.Marshal(result.Files)
	if err != nil {
		return Run{}, err
	}
	versionID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return Run{}, err
	}
	now := time.Now()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).First(&row).Error; err != nil {
			return err
		}
		version := sourceVersionRow{ID: versionID, RunID: runID, AppID: row.AppID, GenerationType: result.GenerationType, FilesJSON: filesJSON, CreatedAt: now}
		if row.Status == task.StatusPausing {
			return &task.Suspended{Status: task.StatusPaused}
		}
		if row.Status != StatusRunning {
			return fmt.Errorf("cannot complete run in status %s", row.Status)
		}
		if result.Validate != nil {
			if err := result.Validate(); err != nil {
				return err
			}
		}
		if result.Publication != nil {
			publication := *result.Publication
			version.SnapshotPath = publication.Path
			version.ContentHash = publication.ContentHash
			version.RestoredFromID = publication.RestoredFromID
			if publication.Claim.AppID != row.AppID {
				return fmt.Errorf("publication app does not match run")
			}
			publication.VersionID = versionID
			if err := publishTx(tx, publication, result.Validate); err != nil {
				return err
			}
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		if err := s.appendEventTx(tx, &row, EventCodeCompleted, map[string]any{"sourceVersionId": strconv.FormatInt(versionID, 10)}); err != nil {
			return err
		}
		row.LastSequence++
		if result.Description != "" {
			if err := s.appendEventTx(tx, &row, EventDescriptionComplete, result.Description); err != nil {
				return err
			}
			row.LastSequence++
		}
		if err := s.appendEventTx(tx, &row, EventRunCompleted, map[string]any{"status": StatusCompleted}); err != nil {
			return err
		}
		row.LastSequence++
		if err := s.appendEventTx(tx, &row, EventDone, 1); err != nil {
			return err
		}
		return tx.Model(&agentRunRow{}).Where("id = ?", runID).Updates(map[string]any{
			"status": StatusCompleted, "source_version_id": versionID, "active_key": nil,
			"finished_at": now, "updated_at": now,
		}).Error
	})
	if err != nil {
		return Run{}, err
	}
	return s.GetRunInternal(ctx, runID)
}

func (s *GormStore) Fail(ctx context.Context, runID int64, message string, interrupted bool) (Run, error) {
	status := StatusFailed
	if interrupted {
		status = StatusInterrupted
	}
	now := time.Now()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).First(&row).Error; err != nil {
			return err
		}
		if rowToRun(row).Terminal() {
			return nil
		}
		// Never downgrade a persisted safe-point stop because shutdown raced it.
		if row.Status == task.StatusPaused || row.Status == task.StatusAwaitingApproval || row.Status == task.StatusRecoverable {
			return nil
		}
		if row.Status == task.StatusCancelling {
			status = task.StatusCanceled
		}
		if interrupted && row.Status != task.StatusCancelling {
			var saved model.RunCheckpoint
			err := tx.First(&saved, "run_id = ?", runID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				if err := tx.Model(&row).Updates(map[string]any{"status": task.StatusRecoverable, "updated_at": now}).Error; err != nil {
					return err
				}
				return s.appendEventTx(tx, &row, "run_suspended", map[string]any{"status": task.StatusRecoverable})
			}
		}
		if status != task.StatusCanceled {
			if err := s.appendEventTx(tx, &row, EventError, message); err != nil {
				return err
			}
			row.LastSequence++
		}
		eventType := EventRunFailed
		if status == task.StatusCanceled {
			eventType = "run_canceled"
		}
		if err := s.appendEventTx(tx, &row, eventType, map[string]any{"status": status}); err != nil {
			return err
		}
		row.LastSequence++
		if err := s.appendEventTx(tx, &row, EventDone, 1); err != nil {
			return err
		}
		return tx.Model(&agentRunRow{}).Where("id = ?", runID).Updates(map[string]any{
			"status": status, "error_message": message, "active_key": nil,
			"finished_at": now, "updated_at": now,
		}).Error
	})
	if err != nil {
		return Run{}, err
	}
	return s.GetRunInternal(ctx, runID)
}

func (s *GormStore) ClaimOutbox(ctx context.Context, limit int) ([]outboxMessage, error) {
	var rows []outboxRow
	now := time.Now()
	// A lost delivery acknowledgement must not strand an outbox row forever.
	if err := s.db.WithContext(ctx).Model(&outboxRow{}).Where("status = ? AND claimed_at < ?", "processing", now.Add(-time.Minute)).Updates(map[string]any{"status": "pending", "claimed_at": nil}).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("status = ? AND next_attempt_at <= ?", "pending", now).Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	claimed := make([]outboxMessage, 0, len(rows))
	for _, row := range rows {
		result := s.db.WithContext(ctx).Model(&outboxRow{}).Where("id = ? AND status = ?", row.ID, "pending").Updates(map[string]any{"status": "processing", "claimed_at": now, "updated_at": now})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			claimed = append(claimed, outboxMessage{ID: row.ID, AggregateID: row.AggregateID, EventType: row.EventType, Payload: row.Payload, Attempts: row.Attempts, NextAttemptAt: row.NextAttemptAt})
		}
	}
	return claimed, nil
}

func (s *GormStore) MarkOutboxDelivered(ctx context.Context, id int64) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&outboxRow{}).Where("id = ?", id).Updates(map[string]any{"status": "delivered", "delivered_at": now, "updated_at": now}).Error
}

func (s *GormStore) RetryOutbox(ctx context.Context, id int64, attempts int, lastError string) error {
	delay := time.Duration(attempts*attempts) * time.Second
	if delay > time.Minute {
		delay = time.Minute
	}
	return s.db.WithContext(ctx).Model(&outboxRow{}).Where("id = ?", id).Updates(map[string]any{
		"status": "pending", "attempts": attempts, "last_error": lastError,
		"next_attempt_at": time.Now().Add(delay), "claimed_at": nil, "updated_at": time.Now(),
	}).Error
}

func (s *GormStore) InterruptStale(ctx context.Context) error {
	now := time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []agentRunRow
		if err := tx.Where("status IN ?", []string{StatusPending, StatusRunning, task.StatusPausing, task.StatusCancelling}).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var saved model.RunCheckpoint
			cpErr := tx.First(&saved, "run_id = ?", row.ID).Error
			if cpErr != nil && !errors.Is(cpErr, gorm.ErrRecordNotFound) {
				return cpErr
			}
			if cpErr == nil && row.Status != task.StatusCancelling {
				if err := tx.Model(&agentRunRow{}).Where("id = ?", row.ID).Updates(map[string]any{"status": task.StatusRecoverable, "updated_at": now}).Error; err != nil {
					return err
				}
				if err := s.appendEventTx(tx, &row, "run_suspended", map[string]any{"status": task.StatusRecoverable}); err != nil {
					return err
				}
				continue
			}
			terminalStatus := StatusInterrupted
			if row.Status == task.StatusCancelling {
				terminalStatus = task.StatusCanceled
			}
			failureEvent := EventRunFailed
			errorMessage := "server restarted before the task completed"
			if terminalStatus == task.StatusCanceled {
				failureEvent = "run_canceled"
				errorMessage = ""
			} else {
				if err := s.appendEventTx(tx, &row, EventError, errorMessage); err != nil {
					return err
				}
				row.LastSequence++
			}
			if err := s.appendEventTx(tx, &row, failureEvent, map[string]any{"status": terminalStatus}); err != nil {
				return err
			}
			row.LastSequence++
			if err := s.appendEventTx(tx, &row, EventDone, 1); err != nil {
				return err
			}
			if err := tx.Model(&agentRunRow{}).Where("id = ?", row.ID).Updates(map[string]any{
				"status": terminalStatus, "error_message": errorMessage,
				"active_key": nil, "finished_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&outboxRow{}).Where("event_type = ? AND status IN ?", outboxTaskReady, []string{"pending", "processing"}).Updates(map[string]any{"status": "failed", "last_error": "run interrupted by server restart", "updated_at": now}).Error
	})
}

func (s *GormStore) appendEventTx(tx *gorm.DB, run *agentRunRow, eventType string, payload any) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	eventID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return err
	}
	outboxID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return err
	}
	sequence := run.LastSequence + 1
	now := time.Now()
	event := runEventRow{ID: eventID, RunID: run.ID, Sequence: sequence, EventType: eventType, Payload: payloadJSON, CreatedAt: now}
	if err := tx.Create(&event).Error; err != nil {
		return err
	}
	if err := tx.Model(&agentRunRow{}).Where("id = ?", run.ID).Updates(map[string]any{"last_sequence": sequence, "updated_at": now}).Error; err != nil {
		return err
	}
	outbox := outboxRow{
		ID: outboxID, EventID: fmt.Sprintf("run-event:%d:%d", run.ID, sequence), AggregateType: "agent_run",
		AggregateID: run.ID, EventType: outboxEventAvailable, Payload: []byte(`{}`), Status: "pending",
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return tx.Create(&outbox).Error
}

func rowToRun(row agentRunRow) Run {
	return Run{
		ID: row.ID, AppID: row.AppID, UserID: row.UserID, RequestID: row.RequestID,
		Kind: row.Kind, Status: row.Status, Input: row.InputJSON, SourceVersionID: row.SourceVersionID,
		LastSequence: row.LastSequence, ErrorMessage: row.ErrorMessage, CreatedAt: row.CreatedAt,
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
	}
}
