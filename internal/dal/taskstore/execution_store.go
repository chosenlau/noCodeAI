package taskstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExecutionStore struct{ db *gorm.DB }

func NewExecutionStore(db *gorm.DB) *ExecutionStore { return &ExecutionStore{db: db} }

// All ownership times come from MySQL, so application clock skew cannot extend a lease.
func dbTime(tx *gorm.DB) (time.Time, error) {
	var now time.Time
	err := tx.Raw("SELECT NOW(3)").Scan(&now).Error
	return now, err
}

func (s *ExecutionStore) Claim(ctx context.Context, appID int64, owner string, ttl time.Duration) (execution.Claim, error) {
	var claim execution.Claim
	if appID <= 0 || owner == "" || ttl <= 0 {
		return claim, fmt.Errorf("invalid execution claim")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := dbTime(tx)
		if err != nil {
			return err
		}
		initial := model.AppExecution{AppID: appID, LeaseUntil: now, CurrentPath: ""}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
			return err
		}
		var row model.AppExecution
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("app_id = ?", appID).First(&row).Error; err != nil {
			return err
		}
		now, err = dbTime(tx)
		if err != nil {
			return err
		}
		if row.Owner != "" && row.LeaseUntil.After(now) {
			return execution.ErrBusy
		}
		claim = execution.Claim{AppID: appID, Owner: owner, Epoch: row.Epoch + 1, BaseVersionID: row.CurrentVersionID, BasePath: row.CurrentPath}
		return tx.Model(&row).Updates(map[string]any{"epoch": claim.Epoch, "owner": owner, "lease_until": now.Add(ttl)}).Error
	})
	return claim, err
}

func (s *ExecutionStore) Renew(ctx context.Context, claim execution.Claim, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("invalid lease duration")
	}
	result := s.db.WithContext(ctx).Model(&model.AppExecution{}).Where("app_id = ? AND epoch = ? AND owner = ? AND lease_until > NOW(3)", claim.AppID, claim.Epoch, claim.Owner).Update("lease_until", gorm.Expr("TIMESTAMPADD(MICROSECOND, ?, NOW(3))", ttl.Microseconds()))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return execution.ErrFenced
	}
	return nil
}

func (s *ExecutionStore) Release(ctx context.Context, claim execution.Claim) error {
	return s.db.WithContext(ctx).Model(&model.AppExecution{}).Where("app_id = ? AND epoch = ? AND owner = ?", claim.AppID, claim.Epoch, claim.Owner).Updates(map[string]any{"owner": "", "lease_until": gorm.Expr("NOW(3)")}).Error
}

func (s *ExecutionStore) Current(ctx context.Context, appID int64) (execution.Version, error) {
	var row model.AppExecution
	err := s.db.WithContext(ctx).Where("app_id = ?", appID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return execution.Version{}, nil
	}
	return execution.Version{ID: row.CurrentVersionID, Path: row.CurrentPath}, err
}

// publishTx is shared by legacy publishing and Run completion. The pointer update
// and the Run's snapshot/events therefore commit or roll back together.
func publishTx(tx *gorm.DB, publication execution.Publication, validators ...func() error) error {
	if publication.Path == "" || publication.VersionID <= 0 || publication.Claim.Owner == "" {
		return fmt.Errorf("invalid publication")
	}
	var row model.AppExecution
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("app_id = ?", publication.Claim.AppID).First(&row).Error; err != nil {
		return err
	}
	now, err := dbTime(tx)
	if err != nil {
		return err
	}
	for _, validate := range validators {
		if validate != nil {
			if err := validate(); err != nil {
				return err
			}
		}
	}
	c := publication.Claim
	if row.Epoch != c.Epoch || row.Owner != c.Owner || !row.LeaseUntil.After(now) || row.CurrentVersionID != c.BaseVersionID {
		return execution.ErrFenced
	}
	return tx.Model(&row).Updates(map[string]any{"current_version_id": publication.VersionID, "current_path": publication.Path}).Error
}

func (s *ExecutionStore) Publish(ctx context.Context, publication execution.Publication, files map[string]string, generationType string) (int64, error) {
	if publication.VersionID == 0 {
		id, err := snowflake.GenerateSnowFlakeId()
		if err != nil {
			return 0, err
		}
		publication.VersionID = id
	}
	data, err := json.Marshal(files)
	if err != nil {
		return 0, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := publishTx(tx, publication); err != nil {
			return err
		}
		// Legacy calls have no Run. A unique synthetic run key preserves the existing schema.
		version := model.SourceVersion{ID: publication.VersionID, RunID: publication.VersionID, AppID: publication.Claim.AppID, GenerationType: generationType, FilesJSON: data, CreatedAt: time.Now(), SnapshotPath: publication.Path, ContentHash: publication.ContentHash, RestoredFromID: publication.RestoredFromID}
		return tx.Create(&version).Error
	})
	return publication.VersionID, err
}

func (s *ExecutionStore) SourceVersion(ctx context.Context, appID, id int64) (model.SourceVersion, error) {
	var row model.SourceVersion
	err := s.db.WithContext(ctx).Where("app_id = ? AND id = ?", appID, id).First(&row).Error
	return row, err
}

func (s *ExecutionStore) Versions(ctx context.Context, appID int64) ([]model.SourceVersion, error) {
	var rows []model.SourceVersion
	err := s.db.WithContext(ctx).Select("id", "app_id", "created_at", "snapshot_path", "content_hash", "restored_from_id").Where("app_id = ?", appID).Order("created_at DESC, id DESC").Limit(50).Find(&rows).Error
	return rows, err
}

// RegisterSnapshot adopts only the current pre-v4 snapshot under the execution fence.
func (s *ExecutionStore) RegisterSnapshot(ctx context.Context, c execution.Claim, path, hash string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.AppExecution
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("app_id = ? AND owner = ? AND epoch = ? AND current_version_id = ? AND lease_until > NOW(3)", c.AppID, c.Owner, c.Epoch, c.BaseVersionID).First(&row).Error; err != nil {
			return execution.ErrFenced
		}
		return tx.Model(&model.SourceVersion{}).Where("app_id = ? AND id = ? AND content_hash = ''", c.AppID, c.BaseVersionID).Updates(map[string]any{"snapshot_path": path, "content_hash": hash}).Error
	})
}

// Restoration uses the same pointer fence, and serializes with active Run reservation.
func (s *ExecutionStore) PublishRestoration(ctx context.Context, p execution.Publication, files map[string]string, generationType string) (int64, error) {
	var id int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runs []model.AgentRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("active_key = ?", strconv.FormatInt(p.Claim.AppID, 10)).Find(&runs).Error; err != nil {
			return err
		}
		if len(runs) != 0 {
			return execution.ErrBusy
		}
		var err error
		id, err = NewExecutionStore(tx).Publish(ctx, p, files, generationType)
		if err != nil {
			return err
		}
		return tx.Model(&model.App{}).Where("id = ?", p.Claim.AppID).Updates(map[string]any{"codeGenType": generationType, "projectArchitecture": ""}).Error
	})
	return id, err
}

func (s *ExecutionStore) BeginTool(ctx context.Context, call execution.ToolCall) (execution.ToolRecord, bool, error) {
	if call.ExecutionID == "" || call.CallID == "" || call.ToolName == "" || call.ArgumentsHash == "" {
		return execution.ToolRecord{}, false, fmt.Errorf("missing tool identity")
	}
	id, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return execution.ToolRecord{}, false, err
	}
	now := time.Now()
	row := model.ToolExecution{ID: id, ExecutionID: call.ExecutionID, CallID: call.CallID, ToolName: call.ToolName, ArgumentsHash: call.ArgumentsHash, Status: "running", CreatedAt: now, UpdatedAt: now}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if result.Error != nil {
		return execution.ToolRecord{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		return execution.ToolRecord{Status: "running"}, true, nil
	}
	row = model.ToolExecution{}
	if err := s.db.WithContext(ctx).Where("execution_id = ? AND call_id = ?", call.ExecutionID, call.CallID).First(&row).Error; err != nil {
		return execution.ToolRecord{}, false, err
	}
	if row.ToolName != call.ToolName || row.ArgumentsHash != call.ArgumentsHash {
		return execution.ToolRecord{}, false, execution.ErrToolConflict
	}
	return execution.ToolRecord{Status: row.Status, Result: row.Result, Error: row.Error}, false, nil
}

func (s *ExecutionStore) FinishTool(ctx context.Context, call execution.ToolCall, record execution.ToolRecord) error {
	if record.Status != "succeeded" && record.Status != "failed" && record.Status != "uncertain" {
		return fmt.Errorf("invalid tool terminal status")
	}
	result := s.db.WithContext(ctx).Model(&model.ToolExecution{}).Where("execution_id = ? AND call_id = ? AND tool_name = ? AND arguments_hash = ? AND status = ?", call.ExecutionID, call.CallID, call.ToolName, call.ArgumentsHash, "running").Updates(map[string]any{"status": record.Status, "result": record.Result, "error": record.Error, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return execution.ErrToolConflict
	}
	return nil
}
