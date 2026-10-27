package execution

import (
	"context"
	"errors"
	"time"
)

var ErrFenced = errors.New("execution ownership expired or superseded")
var ErrBusy = errors.New("project already has an active execution")
var ErrToolConflict = errors.New("tool call identity conflicts with stored input")

type Claim struct {
	AppID         int64
	Owner         string
	Epoch         int64
	BaseVersionID int64
	BasePath      string
}

type Version struct {
	ID   int64
	Path string
}

type Publication struct {
	Claim          Claim
	VersionID      int64
	Path           string
	ContentHash    string
	RestoredFromID int64
}

type PublicationStore interface {
	Claim(context.Context, int64, string, time.Duration) (Claim, error)
	Renew(context.Context, Claim, time.Duration) error
	Release(context.Context, Claim) error
	Current(context.Context, int64) (Version, error)
	Publish(context.Context, Publication, map[string]string, string) (int64, error)
}

type ToolCall struct{ ExecutionID, CallID, ToolName, ArgumentsHash string }
type ToolRecord struct{ Status, Result, Error string }
type Journal interface {
	BeginTool(context.Context, ToolCall) (ToolRecord, bool, error)
	FinishTool(context.Context, ToolCall, ToolRecord) error
}
type journalContextKey struct{}

func WithJournal(ctx context.Context, journal Journal) context.Context {
	return context.WithValue(ctx, journalContextKey{}, journal)
}
func JournalFromContext(ctx context.Context) Journal {
	journal, _ := ctx.Value(journalContextKey{}).(Journal)
	return journal
}
