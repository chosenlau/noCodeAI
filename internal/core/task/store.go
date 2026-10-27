package task

import (
	"context"
	"time"
)

const (
	OutboxTaskReady      = "task_ready"
	OutboxEventAvailable = "event_available"
)

type OutboxMessage struct {
	ID            int64
	AggregateID   int64
	EventType     string
	Payload       []byte
	Attempts      int
	NextAttemptAt time.Time
}

// Store is implemented by the DAL adapter. HTTP callers only use Manager.
type Store interface {
	AcquireRuntime(context.Context) (func(), error)
	CheckRuntime(context.Context) error
	CreateRun(context.Context, StartRequest) (Run, bool, error)
	GetRun(context.Context, int64, int64) (Run, error)
	GetRunInternal(context.Context, int64) (Run, error)
	FindActiveRun(context.Context, int64, int64) (Run, error)
	ListEvents(context.Context, int64, int64) ([]Event, error)
	GetSourceFiles(context.Context, int64) (map[string]string, error)
	MarkRunning(context.Context, int64) (Run, error)
	AppendEvent(context.Context, int64, string, any) (Event, error)
	Complete(context.Context, int64, Result) (Run, error)
	Fail(context.Context, int64, string, bool) (Run, error)
	ClaimOutbox(context.Context, int) ([]OutboxMessage, error)
	MarkOutboxDelivered(context.Context, int64) error
	RetryOutbox(context.Context, int64, int, string) error
	InterruptStale(context.Context) error
}
