package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/myfile"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"github.com/cloudwego/eino/schema"
)

type graphRunInput struct {
	Message         string `json:"message"`
	RequireApproval bool   `json:"requireApproval"`
}

type resumeCheckpointKey struct{}

func graphCompatibility() string {
	// Never include credentials. Prompt contents are the loaded runtime values.
	parts := []string{"workflow-v5", "snapshot-v3", "context-v1", "tools-v1", agent.WorkingContextInstruction(), agent.ProjectModifyInstruction, prompt.GetHtmlPrompt(), prompt.GetMultiFilePrompt(), prompt.GetVuePrompt(), prompt.GetRoutingPrompt(), prompt.GetCodeQualityCheckPrompt()}
	if config.GlobalConfig != nil {
		a := config.GlobalConfig.AI
		parts = append(parts, a.Provider, a.Model, a.BaseURL)
		encoded, _ := json.Marshal(a.Context)
		parts = append(parts, string(encoded))
	}
	return task.PlanHash(task.Plan{Constraints: parts}, 0)
}

func (s *AppService) StartGraphRun(ctx context.Context, req *api.GraphRunCreateRequest, loginUser *api.UserVo) (task.Run, bool, error) {
	if s.taskManager == nil || !s.taskManager.Enabled() {
		return task.Run{}, false, errorutil.SystemError.WithMessage("detached graph task is disabled")
	}
	if req == nil || loginUser == nil || req.AppID <= 0 || strings.TrimSpace(req.Message) == "" || strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 128 {
		return task.Run{}, false, errorutil.ParamsError
	}
	app, err := s.loadAppRecord(ctx, req.AppID)
	if err != nil {
		return task.Run{}, false, err
	}
	if app.UserID != loginUser.ID {
		return task.Run{}, false, errorutil.NotAuthError
	}
	approval := true
	if req.RequireApproval != nil {
		approval = *req.RequireApproval
	}
	input, _ := json.Marshal(graphRunInput{Message: req.Message, RequireApproval: approval})
	run, created, err := s.taskManager.Start(ctx, task.StartRequest{
		Kind: task.KindGraph, AppID: req.AppID, UserID: loginUser.ID,
		RequestID: req.RequestID, Input: input,
	})
	if errors.Is(err, task.ErrActiveRun) {
		return run, false, errorutil.ParamsError.WithMessage(fmt.Sprintf("当前应用正在生成中，活动任务：%d", run.ID))
	}
	if errors.Is(err, task.ErrRequestConflict) {
		return task.Run{}, false, errorutil.ParamsError.WithMessage(err.Error())
	}
	if err != nil {
		return task.Run{}, false, errorutil.SystemError.WithMessage(err.Error())
	}
	return run, created, nil
}

func (s *AppService) GetGraphRun(ctx context.Context, runID int64, loginUser *api.UserVo) (task.Run, error) {
	if s.taskManager == nil || loginUser == nil || runID <= 0 {
		return task.Run{}, errorutil.ParamsError
	}
	run, err := s.taskManager.Get(ctx, runID, loginUser.ID)
	return mapTaskError(run, err)
}

func (s *AppService) GetActiveGraphRun(ctx context.Context, appID int64, loginUser *api.UserVo) (task.Run, error) {
	if s.taskManager == nil || loginUser == nil || appID <= 0 {
		return task.Run{}, errorutil.ParamsError
	}
	run, err := s.taskManager.Active(ctx, appID, loginUser.ID)
	return mapTaskError(run, err)
}

func (s *AppService) SubscribeGraphRun(ctx context.Context, runID, afterSequence int64, loginUser *api.UserVo) (*task.Subscription, error) {
	if s.taskManager == nil || loginUser == nil || runID <= 0 || afterSequence < 0 {
		return nil, errorutil.ParamsError
	}
	subscription, err := s.taskManager.Subscribe(ctx, runID, loginUser.ID, afterSequence)
	if errors.Is(err, task.ErrNotFound) {
		return nil, errorutil.ForbiddenError.WithMessage("task run not found")
	}
	if err != nil {
		return nil, errorutil.SystemError.WithMessage(err.Error())
	}
	return subscription, nil
}

func mapTaskError(run task.Run, err error) (task.Run, error) {
	if errors.Is(err, task.ErrNotFound) {
		return task.Run{}, errorutil.ForbiddenError.WithMessage("task run not found")
	}
	if err != nil {
		return task.Run{}, errorutil.SystemError.WithMessage(err.Error())
	}
	return run, nil
}

func (s *AppService) executeGraphRun(ctx context.Context, run task.Run, emit task.EmitFunc) (task.Result, error) {
	var input graphRunInput
	if err := json.Unmarshal(run.Input, &input); err != nil {
		return task.Result{}, fmt.Errorf("decode graph run input: %w", err)
	}
	cp, err := s.taskManager.Checkpoint(ctx, run.ID)
	if err != nil {
		return task.Result{}, err
	}
	budget := execution.NewBudget(execution.BudgetOptions{})
	if cp != nil {
		if cp.Compatibility != graphCompatibility() {
			return task.Result{}, fmt.Errorf("checkpoint is incompatible with current workflow, prompts, tools or model configuration")
		}
		// A crash may strand a provider reservation. Charge its upper estimate
		// rather than giving the resumed attempt those tokens back.
		used := cp.Budget
		used.Tokens += used.ReservedTokens
		used.EstimatedTokens += used.ReservedTokens
		used.ReservedTokens = 0
		if err := budget.Restore(used, cp.Elapsed); err != nil {
			return task.Result{}, err
		}
		ctx = context.WithValue(ctx, resumeCheckpointKey{}, cp)
	}
	ctx = execution.WithBudget(ctx, budget)
	attemptID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return task.Result{}, err
	}
	ctx, scope, err := s.beginCodeExecution(ctx, run.AppID, run.UserID, strconv.FormatInt(attemptID, 10))
	if err != nil {
		return task.Result{}, err
	}
	transferred := false
	defer func() {
		if !transferred {
			scope.release()
		}
	}()
	started := time.Now()
	elapsed := time.Duration(0)
	if cp != nil {
		elapsed = cp.Elapsed
	}
	budget.Observe(func(used execution.BudgetSnapshot) error {
		writeCtx, done := context.WithTimeout(ctx, 5*time.Second)
		defer done()
		return s.taskManager.UpdateBudget(writeCtx, run.ID, scope.claim, used, elapsed+time.Since(started))
	})
	hooks := &state.ExecutionHooks{}
	if cp != nil {
		hooks.NextNode = cp.NextNode
	}
	hooks.SafePoint = func(pointCtx context.Context, next string, wc *state.WorkFlowContext) error {
		if err := budget.Validate(); err != nil {
			return err
		}
		data, err := wc.Snapshot()
		if err != nil {
			return err
		}
		root, err := myfile.GetCodeOutputRoot()
		if err != nil {
			return err
		}
		id, err := snowflake.GenerateSnowFlakeId()
		if err != nil {
			return err
		}
		path, err := execution.Freeze(pointCtx, scope.workspace, filepath.Join(root, "checkpoints", strconv.FormatInt(run.ID, 10), strconv.FormatInt(id, 10)))
		if err != nil {
			return err
		}
		saved := task.Checkpoint{Claim: scope.claim, NextNode: next, State: data, SnapshotPath: path, BaseVersionID: scope.claim.BaseVersionID, Compatibility: graphCompatibility(), Budget: budget.Snapshot(), Elapsed: elapsed + time.Since(started), Plan: wc.Plan}
		saved.PlanHash = task.PlanHash(saved.Plan, saved.BaseVersionID)
		if cp != nil {
			saved.ApprovedHash = cp.ApprovedHash
		}
		if err := s.taskManager.SaveCheckpoint(pointCtx, run.ID, saved); err != nil {
			return err
		}
		cp = &saved
		current, err := s.taskManager.Get(pointCtx, run.ID, run.UserID)
		if err != nil {
			return err
		}
		if input.RequireApproval && next == "code_generator" && saved.PlanHash != saved.ApprovedHash {
			return &task.Suspended{Status: task.StatusAwaitingApproval}
		}
		if current.Status == task.StatusPausing {
			return &task.Suspended{Status: task.StatusPaused}
		}
		return context.Cause(pointCtx)
	}
	ctx = state.WithExecutionHooks(ctx, hooks)
	var stream *schema.StreamReader[*schema.Message]
	var workflowContext *state.WorkFlowContext
	if cp != nil {
		workflowContext, err = state.Restore(cp.State)
		if err == nil {
			workflowContext.GenerateCodeDir = scope.workspace.Root
			// A restart just after plan saving must still enforce the gate.
			if input.RequireApproval && cp.NextNode == "code_generator" && cp.ApprovedHash != cp.PlanHash {
				return task.Result{}, &task.Suspended{Status: task.StatusAwaitingApproval}
			}
			stream, err = s.simpleWorkflow.ExecuteStream(s.withTokenUsageRecorder(ctx, run.AppID), workflowContext)
		}
	} else {
		stream, workflowContext, err = s.GraphToGenCode(ctx, run.AppID, input.Message, &api.UserVo{ID: run.UserID})
	}
	if err != nil {
		return task.Result{}, err
	}
	defer stream.Close()
	if err := consumeGraphRunStream(stream, emit); err != nil {
		return task.Result{}, err
	}
	if !workflowContext.QualityResult.IsValid {
		message := workflowContext.Description
		if message == "" && len(workflowContext.QualityResult.Errors) > 0 {
			message = workflowContext.QualityResult.Errors[0]
		}
		if message == "" {
			message = "code quality check failed"
		}
		return task.Result{}, errors.New(message)
	}
	publication, files, err := scope.freeze(ctx)
	if err != nil {
		return task.Result{}, err
	}
	transferred = true
	return task.Result{
		Validate: func() error {
			if err := context.Cause(ctx); err != nil {
				return err
			}
			return budget.Validate()
		},
		AfterCommit: func() {
			s.savePublishedWorkflow(ctx, workflowContext, run.AppID, run.UserID)
		},
		Publication: publication,
		Release: func() {
			scope.release()
			s.chatHistoryService.MaybeGenerateSummary(context.Background(), run.AppID, run.UserID)
		},
		Files: files, GenerationType: string(workflowContext.GenerationType),
		Description: workflowContext.Description,
	}, nil
}

func consumeGraphRunStream(stream *schema.StreamReader[*schema.Message], emit task.EmitFunc) error {
	for {
		message, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if message == nil || message.Content == "" {
			continue
		}
		eventType, payload, ok := graphSemanticEvent(message.Content)
		if ok {
			if err := emit(eventType, payload); err != nil {
				return err
			}
		}
	}
}

func graphSemanticEvent(content string) (string, any, bool) {
	var envelope struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(content), &envelope) != nil {
		return "", nil, false
	}
	switch envelope.Type {
	case task.EventStepStarted, task.EventStepCompleted, task.EventToolRequest, task.EventToolExecuted, "context_compacting", "context_ready":
		return envelope.Type, json.RawMessage(content), true
	default:
		return "", nil, false
	}
}
