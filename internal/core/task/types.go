package task

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
)

const (
	KindGraph = "graph"

	StatusPending     = "pending"
	StatusRunning     = "running"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
)

const (
	EventRunCreated          = "run_created"
	EventRunStarted          = "run_started"
	EventRunCompleted        = "run_completed"
	EventRunFailed           = "run_failed"
	EventStepStarted         = "step_started"
	EventStepCompleted       = "step_completed"
	EventToolRequest         = "tool_request"
	EventToolExecuted        = "tool_executed"
	EventCodeCompleted       = "code_completed"
	EventDescriptionComplete = "description_completed"
	EventError               = "error"
	EventDone                = "done"
)

var (
	ErrDisabled        = errors.New("detached graph task is disabled")
	ErrNotFound        = errors.New("task run not found")
	ErrActiveRun       = errors.New("an active task already exists for this app")
	ErrRequestConflict = errors.New("request ID already used with different input")
	ErrStopped         = errors.New("task runtime is stopping")
)

type Options struct {
	Enabled         bool
	WorkerCount     int
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
	OutboxBatchSize int
}

type StartRequest struct {
	Kind      string
	AppID     int64
	UserID    int64
	RequestID string
	Input     json.RawMessage
}

type Run struct {
	ID              int64           `json:"id,string"`
	AppID           int64           `json:"appId,string"`
	UserID          int64           `json:"userId,string"`
	RequestID       string          `json:"requestId"`
	Kind            string          `json:"kind"`
	Status          string          `json:"status"`
	Input           json.RawMessage `json:"-"`
	SourceVersionID *int64          `json:"sourceVersionId,string,omitempty"`
	LastSequence    int64           `json:"lastSequence"`
	ErrorMessage    string          `json:"errorMessage,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	FinishedAt      *time.Time      `json:"finishedAt,omitempty"`
	Checkpoint      *Checkpoint     `json:"checkpoint,omitempty"`
}

func (r Run) Terminal() bool {
	switch r.Status {
	case StatusCompleted, StatusFailed, StatusInterrupted, StatusCanceled:
		return true
	default:
		return false
	}
}

type Event struct {
	RunID     int64           `json:"runId,string"`
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"createdAt"`
}

type Result struct {
	// Validate observes the actual execution context, including lease loss.
	Validate func() error
	// AfterCommit persists best-effort metadata after authoritative publication.
	AfterCommit    func()
	Release        func()
	Publication    *execution.Publication
	Files          map[string]string
	GenerationType string
	Description    string
}

type EmitFunc func(eventType string, payload any) error

type Handler func(ctx context.Context, run Run, emit EmitFunc) (Result, error)
