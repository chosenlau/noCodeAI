package logic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	graphnode "github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal/taskstore"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/myfile"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"github.com/cloudwego/eino/schema"
)

type executionScope struct {
	claim          execution.Claim
	workspace      *execution.Workspace
	store          execution.PublicationStore
	release        func()
	mode           string
	generationType enum.CodeGenTypeEnum
}
type executionScopeKey struct{}

func (s *AppService) beginCodeExecution(ctx context.Context, appID, userID int64, owner string) (context.Context, *executionScope, error) {
	if s.db == nil {
		return nil, nil, fmt.Errorf("execution database is not initialized")
	}
	if owner == "" {
		if s.taskManager != nil && s.taskManager.Enabled() {
			run, err := s.taskManager.Active(ctx, appID, userID)
			if err != nil && !errors.Is(err, task.ErrNotFound) {
				return nil, nil, err
			}
			if run.ID != 0 {
				return nil, nil, fmt.Errorf("application busy; active run %d", run.ID)
			}
		}
		id, err := snowflake.GenerateSnowFlakeId()
		if err != nil {
			return nil, nil, err
		}
		owner = strconv.FormatInt(id, 10)
	}
	locker, ok := s.memoryStore.(store.LeaseLocker)
	if !ok {
		return nil, nil, fmt.Errorf("renewable application lease is required")
	}
	dbStore := taskstore.NewExecutionStore(s.db)
	budget := execution.GetBudget(ctx)
	if budget == nil {
		budget = execution.NewBudget(execution.BudgetOptions{})
	}
	execCtx, lease, err := execution.Acquire(ctx, locker, dbStore, appID, owner, budget)
	if err != nil {
		return nil, nil, err
	}
	claim := lease.Claim
	scope := &executionScope{claim: claim, store: dbStore, release: lease.Release}
	root, err := myfile.GetCodeOutputRoot()
	if err != nil {
		scope.release()
		return nil, nil, err
	}
	source := claim.BasePath
	if resume, _ := ctx.Value(resumeCheckpointKey{}).(*task.Checkpoint); resume != nil {
		if claim.BaseVersionID != resume.BaseVersionID {
			scope.release()
			return nil, nil, fmt.Errorf("project baseline changed; start a new task")
		}
		source = resume.SnapshotPath
	}
	if source == "" {
		app, loadErr := s.loadAppRecord(execCtx, appID)
		if loadErr != nil {
			scope.release()
			return nil, nil, loadErr
		}
		// No pointer exists for an old project. Copy its existing files as the baseline.
		projectRoot, rootErr := myfile.GetProjectRoot()
		if rootErr != nil {
			scope.release()
			return nil, nil, rootErr
		}
		if directory, findErr := findSourceCodeDirectory([]string{root, filepath.Join(projectRoot, "saves")}, appID, enum.CodeGenTypeEnum(app.CodeGenType), ""); findErr == nil {
			source = directory
		} else if !errors.Is(findErr, os.ErrNotExist) {
			scope.release()
			return nil, nil, findErr
		}
	}
	scope.workspace = &execution.Workspace{AppID: appID, ExecutionID: owner, Root: filepath.Join(root, "workspaces", strconv.FormatInt(appID, 10), owner)}
	if err := execution.PrepareWorkspace(execCtx, scope.workspace.Root, source); err != nil {
		scope.release()
		return nil, nil, err
	}
	app, err := s.loadAppRecord(execCtx, appID)
	if err != nil {
		scope.release()
		return nil, nil, err
	}
	scope.generationType, _, err = execution.DetectSourceType(scope.workspace.Root, enum.CodeGenTypeEnum(app.CodeGenType))
	if err != nil {
		scope.release()
		return nil, nil, err
	}
	scope.mode = state.ModeBootstrap
	if scope.generationType != "" {
		scope.mode = state.ModeModify
	}
	// A current version is authoritative even while app metadata is a lagging projection.
	if claim.BaseVersionID != 0 {
		version, loadErr := dbStore.SourceVersion(execCtx, appID, claim.BaseVersionID)
		if loadErr != nil {
			scope.release()
			return nil, nil, loadErr
		}
		if enum.CodeGenTypeTextMap[enum.CodeGenTypeEnum(version.GenerationType)] == "" {
			scope.release()
			return nil, nil, fmt.Errorf("unsupported baseline generation type")
		}
		scope.generationType = enum.CodeGenTypeEnum(version.GenerationType)
		scope.mode = state.ModeModify
		// Adopt pre-iteration-4 current snapshots once, without inventing paths for old history.
		hash, hashErr := execution.SourceHash(execCtx, claim.BasePath)
		if hashErr != nil {
			scope.release()
			return nil, nil, hashErr
		}
		if version.ContentHash != "" && version.ContentHash != hash {
			scope.release()
			return nil, nil, fmt.Errorf("project snapshot changed unexpectedly")
		}
		if resume, _ := ctx.Value(resumeCheckpointKey{}).(*task.Checkpoint); resume == nil {
			copiedHash, copyErr := execution.SourceHash(execCtx, scope.workspace.Root)
			if copyErr != nil || copiedHash != hash {
				scope.release()
				return nil, nil, fmt.Errorf("copied project baseline failed integrity check: %v", copyErr)
			}
		}
		if version.ContentHash == "" {
			if err := dbStore.RegisterSnapshot(execCtx, claim, claim.BasePath, hash); err != nil {
				scope.release()
				return nil, nil, err
			}
		}
	} else if resume, _ := ctx.Value(resumeCheckpointKey{}).(*task.Checkpoint); resume == nil && scope.mode == state.ModeModify {
		// Import legacy files as the first recoverable source version before editing.
		path, freezeErr := execution.Freeze(execCtx, scope.workspace, filepath.Join(root, "versions", strconv.FormatInt(appID, 10), owner+"-baseline"))
		if freezeErr != nil {
			scope.release()
			return nil, nil, freezeErr
		}
		hash, hashErr := execution.SourceHash(execCtx, path)
		if hashErr != nil {
			scope.release()
			return nil, nil, hashErr
		}
		files, _, readErr := graphnode.ReadCodeFiles(path)
		if readErr != nil {
			scope.release()
			return nil, nil, readErr
		}
		id, publishErr := dbStore.Publish(execCtx, execution.Publication{Claim: claim, Path: path, ContentHash: hash}, files, string(scope.generationType))
		if publishErr != nil {
			scope.release()
			return nil, nil, publishErr
		}
		scope.claim.BaseVersionID, scope.claim.BasePath = id, path
	}
	execCtx = execution.WithWorkspace(execCtx, scope.workspace)
	execCtx = execution.WithJournal(execCtx, dbStore)
	return context.WithValue(execCtx, executionScopeKey{}, scope), scope, nil
}

func (s *executionScope) freeze(ctx context.Context) (*execution.Publication, map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	root, err := myfile.GetCodeOutputRoot()
	if err != nil {
		return nil, nil, err
	}
	path, err := execution.Freeze(ctx, s.workspace, filepath.Join(root, "versions", strconv.FormatInt(s.claim.AppID, 10), s.claim.Owner))
	if err != nil {
		return nil, nil, err
	}
	files, _, err := graphnode.ReadCodeFiles(path)
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	hash, err := execution.SourceHash(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return &execution.Publication{Claim: s.claim, Path: path, ContentHash: hash}, files, nil
}

// finishExecutionStream keeps the lease until all source consumers and publishing
// finish. Legacy requests retain their request context; detached Runs use the
// Runtime context and transfer release to Manager instead.
func finishExecutionStream(ctx context.Context, source *schema.StreamReader[*schema.Message], finish func() error, release func()) *schema.StreamReader[*schema.Message] {
	reader, writer := schema.Pipe[*schema.Message](2)
	// Cancellation must also unblock backpressure when the observer stops reading.
	reader.SetAutomaticClose()
	source.SetAutomaticClose()
	stopClosing := context.AfterFunc(ctx, func() {
		reader.Close()
		source.Close()
	})
	go func() {
		defer stopClosing()
		defer release()
		defer source.Close()
		defer writer.Close()
		for {
			msg, err := source.Recv()
			if err == io.EOF {
				if ctx.Err() != nil {
					_ = writer.Send(nil, context.Cause(ctx))
					return
				}
				if finish != nil {
					if err := finish(); err != nil {
						_ = writer.Send(nil, err)
					}
				}
				return
			}
			if err != nil {
				_ = writer.Send(nil, err)
				return
			}
			if writer.Send(msg, nil) {
				return
			}
		}
	}()
	return reader
}

func (s *AppService) GraphToGenCode(ctx context.Context, appID int64, message string, user *api.UserVo) (*schema.StreamReader[*schema.Message], *state.WorkFlowContext, error) {
	if scope, _ := ctx.Value(executionScopeKey{}).(*executionScope); scope != nil {
		if scope.claim.AppID != appID {
			return nil, nil, fmt.Errorf("execution app mismatch")
		}
		return s.graphToGenCode(ctx, appID, message, user)
	}
	if user == nil || appID <= 0 || strings.TrimSpace(message) == "" {
		return nil, nil, fmt.Errorf("invalid app, user or message")
	}
	app, err := s.loadAppRecord(ctx, appID)
	if err != nil {
		return nil, nil, err
	}
	if app.UserID != user.ID {
		return nil, nil, fmt.Errorf("permission denied")
	}
	execCtx, scope, err := s.beginCodeExecution(ctx, appID, user.ID, "")
	if err != nil {
		return nil, nil, err
	}
	source, wc, err := s.graphToGenCode(execCtx, appID, message, user)
	if err != nil {
		scope.release()
		return nil, nil, err
	}
	return finishExecutionStream(execCtx, source, func() error {
		if !wc.QualityResult.IsValid {
			return fmt.Errorf("quality check failed")
		}
		pub, files, err := scope.freeze(execCtx)
		if err != nil {
			return err
		}
		_, err = scope.store.Publish(execCtx, *pub, files, string(wc.GenerationType))
		if err == nil {
			s.savePublishedWorkflow(execCtx, wc, appID, user.ID)
		}
		return err
	}, func() {
		scope.release()
		s.chatHistoryService.MaybeGenerateSummary(context.Background(), appID, user.ID)
	}), wc, nil
}

func (s *AppService) ChatToGenCode(ctx context.Context, appID int64, message string, user *api.UserVo) (*schema.StreamReader[*schema.Message], error) {
	if user == nil || appID <= 0 || strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("invalid app, user or message")
	}
	app, err := s.loadAppRecord(ctx, appID)
	if err != nil {
		return nil, err
	}
	if app.UserID != user.ID {
		return nil, fmt.Errorf("permission denied")
	}
	execCtx, scope, err := s.beginCodeExecution(ctx, appID, user.ID, "")
	if err != nil {
		return nil, err
	}
	source, err := s.chatToGenCode(context.WithValue(execCtx, "appId", appID), appID, message, user)
	if err != nil {
		scope.release()
		return nil, err
	}
	return finishExecutionStream(execCtx, source, func() error {
		pub, files, err := scope.freeze(execCtx)
		if err != nil {
			return err
		}
		generationType := app.CodeGenType
		if scope.mode == state.ModeModify {
			generationType = string(scope.generationType)
		}
		_, err = scope.store.Publish(execCtx, *pub, files, generationType)
		return err
	}, func() {
		scope.release()
		s.chatHistoryService.MaybeGenerateSummary(context.Background(), appID, user.ID)
	}), nil
}

func (s *AppService) ChatWithAgent(ctx context.Context, appID int64, message string, user *api.UserVo) (*schema.StreamReader[*schema.Message], error) {
	if user == nil || appID <= 0 || strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("invalid app, user or message")
	}
	app, err := s.loadAppRecord(ctx, appID)
	if err != nil {
		return nil, err
	}
	if app.UserID != user.ID {
		return nil, fmt.Errorf("permission denied")
	}
	if s.db == nil {
		return nil, fmt.Errorf("execution database is not initialized")
	}
	locks, ok := s.memoryStore.(store.LeaseLocker)
	if !ok {
		return nil, fmt.Errorf("renewable application lease is required")
	}
	if s.taskManager != nil && s.taskManager.Enabled() {
		run, err := s.taskManager.Active(ctx, appID, user.ID)
		if err != nil && !errors.Is(err, task.ErrNotFound) {
			return nil, err
		}
		if run.ID != 0 {
			return nil, fmt.Errorf("application busy; active run %d", run.ID)
		}
	}
	id, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return nil, err
	}
	execCtx, lease, err := execution.Acquire(ctx, locks, taskstore.NewExecutionStore(s.db), appID, strconv.FormatInt(id, 10), execution.NewBudget(execution.BudgetOptions{}))
	if err != nil {
		return nil, err
	}
	source, err := s.chatWithAgent(execCtx, appID, message, user)
	if err != nil {
		lease.Release()
		return nil, err
	}
	return finishExecutionStream(execCtx, source, nil, func() {
		lease.Release()
		s.chatHistoryService.MaybeGenerateSummary(context.Background(), appID, user.ID)
	}), nil
}
