package execution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBudgetConcurrentLimits(t *testing.T) {
	b := NewBudget(BudgetOptions{ToolCalls: 7})
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			if b.ConsumeTool() == nil {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 7 || b.Snapshot().ToolCalls != 7 {
		t.Fatalf("budget escaped under concurrent use: %+v", b.Snapshot())
	}
}

func TestBudgetReservationsSettleOnce(t *testing.T) {
	b := NewBudget(BudgetOptions{Tokens: 100, OutputTokens: 60})
	limit, finish, err := b.ReserveModel(20)
	if err != nil || limit != 60 {
		t.Fatalf("reservation: %d %v", limit, err)
	}
	if _, _, err = b.ReserveModel(20); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("concurrent reserved tokens not protected: %v", err)
	}
	finish(30)
	finish(30)
	if s := b.Snapshot(); s.Tokens != 30 || s.ReservedTokens != 0 {
		t.Fatalf("bad settlement: %+v", s)
	}
	limit, finish, err = b.ReserveModel(20)
	if err != nil || limit != 50 {
		t.Fatalf("remaining output: %d %v", limit, err)
	}
	finish(-1)
	if b.Snapshot().Tokens != 100 {
		t.Fatal("missing provider usage must charge reservation")
	}
	if !errors.Is(b.ConsumeTool(), ErrBudgetExceeded) {
		t.Fatal("exhausted tokens must stop further tools")
	}
}

func TestModelSlotsRespectCancellation(t *testing.T) {
	var releases []func()
	for range cap(modelSlots) {
		release, err := AcquireModel(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
			release()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := AcquireModel(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("capacity wait ignored context: %v", err)
	}
}

func TestBudgetRestoreKeepsUsageAndElapsed(t *testing.T) {
	b := NewBudget(BudgetOptions{ToolCalls: 2, Timeout: time.Minute})
	if err := b.Restore(BudgetSnapshot{ToolCalls: 1, Tokens: 40}, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(b.Deadline()); remaining <= 0 || remaining > 30*time.Second {
		t.Fatalf("elapsed budget was reset: %v", remaining)
	}
	if err := b.ConsumeTool(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(b.ConsumeTool(), ErrBudgetExceeded) || b.Snapshot().Tokens != 40 {
		t.Fatal("resume reset consumed budget")
	}
	if err := b.Restore(BudgetSnapshot{ReservedTokens: 1}, 0); err == nil {
		t.Fatal("unsettled reservation must be normalized before restore")
	}
	if !errors.Is(b.Restore(BudgetSnapshot{}, time.Minute), ErrBudgetExceeded) {
		t.Fatal("exhausted time must reject resume")
	}
}

func TestBudgetPersistenceFailureStopsFurtherWork(t *testing.T) {
	b := NewBudget(BudgetOptions{})
	storageErr := errors.New("checkpoint storage unavailable")
	var writes int
	b.Observe(func(snapshot BudgetSnapshot) error {
		writes++
		if snapshot.ReservedTokens == 0 {
			return storageErr // Reservation persisted, settlement failed.
		}
		return nil
	})
	_, finish, err := b.ReserveModel(20)
	if err != nil {
		t.Fatal(err)
	}
	finish(30)
	if !errors.Is(b.Validate(), storageErr) || !errors.Is(b.ConsumeTool(), storageErr) {
		t.Fatal("failed settlement must block publication and further tools")
	}
	if writes != 2 {
		t.Fatalf("unexpected durable writes: %d", writes)
	}
}
