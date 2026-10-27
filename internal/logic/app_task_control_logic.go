package logic

import (
	"context"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal/taskstore"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
)

func (s *AppService) ControlGraphRun(ctx context.Context, runID int64, action, hash string, user *api.UserVo) (task.Run, error) {
	if s.taskManager == nil || user == nil || runID <= 0 {
		return task.Run{}, errorutil.ParamsError
	}
	if action == "approve" || action == "resume" {
		run, err := s.taskManager.Get(ctx, runID, user.ID)
		if err != nil {
			return mapTaskError(run, err)
		}
		cp := run.Checkpoint
		if cp == nil || cp.Compatibility != graphCompatibility() {
			return task.Run{}, errorutil.ParamsError.WithMessage("checkpoint is incompatible with current workflow, prompts, tools or model configuration")
		}
		if s.db == nil {
			return task.Run{}, errorutil.SystemError.WithMessage("execution database is not initialized")
		}
		current, err := taskstore.NewExecutionStore(s.db).Current(ctx, run.AppID)
		if err != nil {
			return task.Run{}, errorutil.SystemError.WithMessage(err.Error())
		}
		if current.ID != cp.BaseVersionID {
			return task.Run{}, errorutil.ParamsError.WithMessage("project baseline changed; cancel this task and create a new plan")
		}
		used := cp.Budget
		used.Tokens += used.ReservedTokens
		used.ReservedTokens = 0
		if err := execution.NewBudget(execution.BudgetOptions{}).Restore(used, cp.Elapsed); err != nil {
			return task.Run{}, errorutil.ParamsError.WithMessage(err.Error())
		}
	}
	run, err := s.taskManager.Control(ctx, runID, user.ID, action, hash)
	if err == task.ErrInvalidControl {
		return task.Run{}, errorutil.ParamsError.WithMessage(err.Error())
	}
	return mapTaskError(run, err)
}
