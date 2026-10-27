package taskstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Update only resource accounting, preserving the last consistent node/files.
func (s *GormStore) UpdateBudget(ctx context.Context, id int64, claim execution.Claim, used execution.BudgetSnapshot, elapsed time.Duration) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", id).Error; err != nil {
			return err
		}
		if run.Status != StatusRunning && run.Status != task.StatusPausing {
			return task.ErrInvalidControl
		}
		if err := checkpointFence(tx, run.AppID, claim); err != nil {
			return err
		}
		var saved model.RunCheckpoint
		err := tx.First(&saved, "run_id = ?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		cp, err := task.DecodeCheckpoint(saved.Document)
		if err != nil {
			return err
		}
		cp.Budget, cp.Elapsed = used, elapsed
		data, err := cp.Encode()
		if err != nil {
			return err
		}
		return tx.Model(&saved).Updates(map[string]any{"document": data, "updated_at": time.Now()}).Error
	})
}

func checkpointFence(tx *gorm.DB, appID int64, claim execution.Claim) error {
	if claim.AppID != appID {
		return execution.ErrFenced
	}
	var row model.AppExecution
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("app_id = ? AND owner = ? AND epoch = ? AND lease_until > NOW(3)", appID, claim.Owner, claim.Epoch).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return execution.ErrFenced
	}
	return err
}

func (s *GormStore) LoadCheckpoint(ctx context.Context, id int64) (*task.Checkpoint, error) {
	var row model.RunCheckpoint
	err := s.db.WithContext(ctx).Where("run_id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return task.DecodeCheckpoint(row.Document)
}

func (s *GormStore) SaveCheckpoint(ctx context.Context, id int64, cp task.Checkpoint) error {
	data, err := cp.Encode()
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", id).Error; err != nil {
			return err
		}
		if run.Status != StatusRunning && run.Status != task.StatusPausing {
			return task.ErrInvalidControl
		}
		if err := checkpointFence(tx, run.AppID, cp.Claim); err != nil {
			return err
		}
		row := model.RunCheckpoint{RunID: id, Document: data, UpdatedAt: time.Now()}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
		return s.appendEventTx(tx, &run, "checkpoint_saved", cp)
	})
}

// Suspend is called only after the handler released its workspace lease.
func (s *GormStore) Suspend(ctx context.Context, id int64, status string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run agentRunRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ?", id).Error; err != nil {
			return err
		}
		if run.Status == task.StatusCancelling {
			return task.ErrInvalidControl
		}
		if run.Status != StatusRunning && run.Status != task.StatusPausing {
			return task.ErrInvalidControl
		}
		if status != task.StatusPaused && status != task.StatusAwaitingApproval {
			return task.ErrInvalidControl
		}
		if err := tx.Model(&run).Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.appendEventTx(tx, &run, "run_suspended", map[string]any{"status": status})
	})
}

func (s *GormStore) Control(ctx context.Context, id, user int64, action, hash string) (task.Run, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run agentRunRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&run, "id = ? AND user_id = ?", id, user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return task.ErrNotFound
		}
		if err != nil {
			return err
		}
		status := run.Status
		updates := map[string]any{"updated_at": time.Now()}
		switch action {
		case "pause":
			if status == task.StatusPausing || status == task.StatusPaused {
				return nil
			}
			if status != StatusRunning {
				return task.ErrInvalidControl
			}
			updates["status"] = task.StatusPausing
		case "cancel", "reject":
			if status == task.StatusCanceled || status == task.StatusCancelling {
				return nil
			}
			if rowToRun(run).Terminal() {
				return task.ErrInvalidControl
			}
			if status == StatusRunning || status == task.StatusPausing {
				updates["status"] = task.StatusCancelling
			} else {
				updates["status"], updates["active_key"], updates["finished_at"] = task.StatusCanceled, nil, time.Now()
			}
		case "approve", "resume":
			var saved model.RunCheckpoint
			if err := tx.First(&saved, "run_id = ?", id).Error; err != nil {
				return task.ErrInvalidControl
			}
			cp, err := task.DecodeCheckpoint(saved.Document)
			if err != nil {
				return err
			}
			if action == "approve" {
				if hash == "" || hash != cp.PlanHash {
					return task.ErrInvalidControl
				}
				if (status == StatusPending || status == StatusRunning) && cp.ApprovedHash == hash {
					return nil
				}
				if status != task.StatusAwaitingApproval {
					return task.ErrInvalidControl
				}
				cp.ApprovedHash = hash
			} else {
				if status == StatusPending {
					return nil
				}
				if status != task.StatusPaused && status != task.StatusRecoverable {
					return task.ErrInvalidControl
				}
			}
			if len(cp.State) == 0 || cp.SnapshotPath == "" {
				return task.ErrInvalidControl
			}
			// Retain/reacquire the unique project reservation across safe-point stops.
			updates["status"], updates["active_key"], updates["error_message"], updates["finished_at"] = StatusPending, fmt.Sprint(run.AppID), "", nil
			data, err := cp.Encode()
			if err != nil {
				return err
			}
			if err := tx.Model(&saved).Update("document", data).Error; err != nil {
				return err
			}
			outboxID, err := snowflake.GenerateSnowFlakeId()
			if err != nil {
				return err
			}
			now := time.Now()
			if err := tx.Create(&outboxRow{ID: outboxID, EventID: fmt.Sprintf("task-resume:%d", outboxID), AggregateType: "agent_run", AggregateID: id, EventType: outboxTaskReady, Payload: []byte(`{}`), Status: "pending", NextAttemptAt: now, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				return err
			}
		default:
			return task.ErrInvalidControl
		}
		if err := tx.Model(&run).Updates(updates).Error; err != nil {
			return err
		}
		if err := s.appendEventTx(tx, &run, "run_control", map[string]any{"action": action, "status": updates["status"]}); err != nil {
			return err
		}
		if updates["status"] == task.StatusCanceled {
			run.LastSequence++
			return s.appendEventTx(tx, &run, EventDone, 1)
		}
		return nil
	})
	if err != nil {
		return task.Run{}, err
	}
	return s.GetRun(ctx, id, user)
}
