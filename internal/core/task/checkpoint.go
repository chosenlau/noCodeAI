package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
)

const (
	StatusPausing          = "pausing"
	StatusPaused           = "paused"
	StatusAwaitingApproval = "awaiting_approval"
	StatusCancelling       = "cancelling"
	StatusCanceled         = "canceled"
	StatusRecoverable      = "recoverable"
)

var ErrInvalidControl = errors.New("invalid task control or stale plan")

type executionHandle struct{ cancel context.CancelFunc }

// Suspended is a successful safe-point stop, not an execution failure.
type Suspended struct{ Status string }

func (e *Suspended) Error() string { return "task suspended: " + e.Status }

type Todo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}
type Plan struct {
	Goal           string   `json:"goal"`
	GenerationType string   `json:"generationType"`
	ExecutionMode  string   `json:"executionMode"`
	Constraints    []string `json:"constraints"`
	Acceptance     []string `json:"acceptance"`
	Todos          []Todo   `json:"todos"`
}

// Only durable business data crosses this interface. No callbacks or streams.
type Checkpoint struct {
	Claim         execution.Claim          `json:"-"`
	NextNode      string                   `json:"nextNode"`
	State         json.RawMessage          `json:"-"`
	SnapshotPath  string                   `json:"-"`
	BaseVersionID int64                    `json:"baseVersionId,string"`
	Compatibility string                   `json:"compatibility"`
	Budget        execution.BudgetSnapshot `json:"budget"`
	Elapsed       time.Duration            `json:"elapsed"`
	Plan          Plan                     `json:"plan"`
	PlanHash      string                   `json:"planHash"`
	ApprovedHash  string                   `json:"-"`
}

// Disk JSON retains private state, while HTTP responses expose only the plan.
type CheckpointData struct {
	Checkpoint
	StateJSON json.RawMessage `json:"state"`
	Path      string          `json:"snapshotPath"`
	Approval  string          `json:"approvedHash"`
}

func (c Checkpoint) Encode() ([]byte, error) {
	return json.Marshal(CheckpointData{Checkpoint: c, StateJSON: c.State, Path: c.SnapshotPath, Approval: c.ApprovedHash})
}
func DecodeCheckpoint(data []byte) (*Checkpoint, error) {
	var d CheckpointData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	d.State, d.SnapshotPath, d.ApprovedHash = d.StateJSON, d.Path, d.Approval
	return &d.Checkpoint, nil
}
func PlanHash(p Plan, base int64) string {
	// Todo progress changes during execution and does not invalidate approval.
	p.Todos = append([]Todo(nil), p.Todos...)
	for i := range p.Todos {
		p.Todos[i].Status = ""
	}
	b, _ := json.Marshal(struct {
		Plan Plan
		Base int64
	}{p, base})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Optional extension keeps existing task-store adapters compatible.
type ControlStore interface {
	LoadCheckpoint(context.Context, int64) (*Checkpoint, error)
	SaveCheckpoint(context.Context, int64, Checkpoint) error
	Suspend(context.Context, int64, string) error
	Control(context.Context, int64, int64, string, string) (Run, error)
	UpdateBudget(context.Context, int64, execution.Claim, execution.BudgetSnapshot, time.Duration) error
}

func (m *Manager) UpdateBudget(ctx context.Context, id int64, claim execution.Claim, used execution.BudgetSnapshot, elapsed time.Duration) error {
	s, ok := m.store.(ControlStore)
	if !ok {
		return ErrDisabled
	}
	return s.UpdateBudget(ctx, id, claim, used, elapsed)
}

func (m *Manager) Checkpoint(ctx context.Context, id int64) (*Checkpoint, error) {
	s, ok := m.store.(ControlStore)
	if !ok {
		return nil, ErrDisabled
	}
	return s.LoadCheckpoint(ctx, id)
}
func (m *Manager) SaveCheckpoint(ctx context.Context, id int64, cp Checkpoint) error {
	s, ok := m.store.(ControlStore)
	if !ok {
		return ErrDisabled
	}
	return s.SaveCheckpoint(ctx, id, cp)
}
func (m *Manager) Control(ctx context.Context, id, user int64, action, hash string) (Run, error) {
	if !m.Enabled() {
		return Run{}, ErrDisabled
	}
	s, ok := m.store.(ControlStore)
	if !ok {
		return Run{}, ErrDisabled
	}
	r, err := s.Control(ctx, id, user, action, hash)
	if err == nil {
		m.mu.Lock()
		if (action == "cancel" || action == "reject") && m.executions[id] != nil {
			m.executions[id].cancel()
		}
		m.mu.Unlock()
		m.hub.notify(id)
		m.notifyWorker()
	}
	return r, err
}
