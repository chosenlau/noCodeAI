package logic

import (
	"context"
	"fmt"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/dal/taskstore"
	"github.com/chosenlau/noCodeAI/pkg/enum"
)

func (s *AppService) ListSourceVersions(ctx context.Context, appID int64, user *api.UserVo) (api.SourceVersionsVo, error) {
	result := api.SourceVersionsVo{Versions: []api.SourceVersionVo{}}
	app, err := s.loadAppRecord(ctx, appID)
	if err != nil {
		return result, err
	}
	if user == nil || app.UserID != user.ID {
		return result, fmt.Errorf("permission denied")
	}
	store := taskstore.NewExecutionStore(s.db)
	current, err := store.Current(ctx, appID)
	if err != nil {
		return result, err
	}
	result.CurrentVersionID = current.ID
	rows, err := store.Versions(ctx, appID)
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		result.Versions = append(result.Versions, api.SourceVersionVo{ID: row.ID, CreatedAt: row.CreatedAt, Current: current.ID == row.ID, Restorable: row.SnapshotPath != "" && row.ContentHash != "", RestoredFromID: row.RestoredFromID})
	}
	return result, nil
}

// Restore creates a fresh source version; historical snapshots are never edited.
func (s *AppService) RestoreSourceVersion(ctx context.Context, appID int64, req api.RestoreSourceRequest, user *api.UserVo) (int64, error) {
	if user == nil || appID <= 0 || req.VersionID <= 0 || req.ExpectedVersionID <= 0 {
		return 0, fmt.Errorf("invalid restore request")
	}
	app, err := s.loadAppRecord(ctx, appID)
	if err != nil {
		return 0, err
	}
	if app.UserID != user.ID {
		return 0, fmt.Errorf("permission denied")
	}
	store := taskstore.NewExecutionStore(s.db)
	target, err := store.SourceVersion(ctx, appID, req.VersionID)
	if err != nil {
		return 0, err
	}
	if target.SnapshotPath == "" || target.ContentHash == "" {
		return 0, fmt.Errorf("此历史版本未保留完整快照，无法恢复")
	}
	if enum.CodeGenTypeTextMap[enum.CodeGenTypeEnum(target.GenerationType)] == "" {
		return 0, fmt.Errorf("unsupported version type")
	}
	execCtx, scope, err := s.beginCodeExecution(ctx, appID, user.ID, "")
	if err != nil {
		return 0, err
	}
	defer scope.release()
	if scope.claim.BaseVersionID != req.ExpectedVersionID {
		return 0, fmt.Errorf("网站已更新，请刷新历史版本后重试")
	}
	if req.VersionID == scope.claim.BaseVersionID {
		return 0, fmt.Errorf("已经是当前版本")
	}
	hash, err := execution.SourceHash(execCtx, target.SnapshotPath)
	if err != nil {
		return 0, err
	}
	if hash != target.ContentHash {
		return 0, fmt.Errorf("历史版本快照校验失败")
	}
	workspace := *scope.workspace
	workspace.Root += "-restore"
	if err := execution.PrepareWorkspace(execCtx, workspace.Root, target.SnapshotPath); err != nil {
		return 0, err
	}
	scope.workspace = &workspace
	pub, files, err := scope.freeze(execCtx)
	if err != nil {
		return 0, err
	}
	if pub.ContentHash != target.ContentHash {
		return 0, fmt.Errorf("复制后的版本校验失败")
	}
	pub.RestoredFromID = target.ID
	return store.PublishRestoration(execCtx, *pub, files, target.GenerationType)
}
