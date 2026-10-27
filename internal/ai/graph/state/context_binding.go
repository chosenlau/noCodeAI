package state

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/chosenlau/noCodeAI/internal/core/task"
)

// BindWorkingContext reuses the same node safe-point persistence as pause/resume.
// Calls happen between complete tool groups with no model request in flight.
func BindWorkingContext(ctx context.Context, w *WorkFlowContext) context.Context {
	if w.WorkingSummaries == nil {
		w.WorkingSummaries = map[string]contextmgr.Snapshot{}
	}
	save := func(ctx context.Context) error {
		if hooks := GetExecutionHooks(ctx); hooks != nil && hooks.SafePoint != nil {
			next := w.NextNode
			if next == "" {
				next = "router"
			}
			return hooks.SafePoint(ctx, next, w)
		}
		return nil
	}
	return contextmgr.WithBinding(ctx, &contextmgr.Binding{
		Load: func(name string) contextmgr.Snapshot { return w.WorkingSummaries[name] },
		Protected: func() string {
			b, _ := json.Marshal(struct {
				Plan task.Plan
				Mode string
				Type string
			}{w.Plan, w.ExecutionMode, string(w.GenerationType)})
			return string(b)
		},
		Commit: func(ctx context.Context, name string, next contextmgr.Snapshot) error {
			old, exists := w.WorkingSummaries[name]
			w.WorkingSummaries[name] = next
			if err := save(ctx); err != nil {
				// A deliberate safe-point suspension has already persisted next.
				if _, ok := err.(*task.Suspended); !ok {
					if exists {
						w.WorkingSummaries[name] = old
					} else {
						delete(w.WorkingSummaries, name)
					}
				}
				return err
			}
			return nil
		},
		Blocked: func(ctx context.Context) error {
			if err := save(ctx); err != nil {
				return fmt.Errorf("%w: %w", contextmgr.ErrCommitFailed, err)
			}
			if hooks := GetExecutionHooks(ctx); hooks != nil && hooks.SafePoint != nil {
				return &task.Suspended{Status: task.StatusPaused}
			}
			return contextmgr.ErrTooLarge
		},
		Notify: func(kind string) {
			if w.StreamChunkCallback != nil {
				b, _ := json.Marshal(map[string]string{"type": kind})
				w.StreamChunkCallback(string(b))
			}
		},
	})
}
