package execution

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"
)

type LeaseSource interface {
	AcquireLease(context.Context, string) (context.Context, func(), error)
}

type Lease struct {
	Claim   Claim
	release func()
}

func (l *Lease) Release() { l.release() }

// Acquire is the shared entry for generation and memory writers. Both Redis
// ownership and the SQL publication lease must be acquired; failure is closed.
func Acquire(ctx context.Context, locks LeaseSource, persistence PublicationStore, appID int64, owner string, budget *Budget) (context.Context, *Lease, error) {
	if locks == nil || persistence == nil || budget == nil {
		return nil, nil, fmt.Errorf("execution coordinator dependencies are missing")
	}
	ctx, cancel := context.WithDeadline(WithBudget(ctx, budget), budget.Deadline())
	redisCtx, unlock, err := locks.AcquireLease(ctx, strconv.FormatInt(appID, 10))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	claim, err := persistence.Claim(redisCtx, appID, owner, 45*time.Second)
	if err != nil {
		unlock()
		cancel()
		return nil, nil, err
	}
	execCtx, stop := context.WithCancelCause(redisCtx)
	var once sync.Once
	lease := &Lease{Claim: claim}
	lease.release = func() {
		once.Do(func() {
			stop(context.Canceled)
			cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
			defer done()
			if err := persistence.Release(cleanup, claim); err != nil {
				log.Printf("release execution %s: %v", owner, err)
			}
			unlock()
			cancel()
		})
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-execCtx.Done():
				return
			case <-ticker.C:
				check, done := context.WithTimeout(execCtx, 3*time.Second)
				err := persistence.Renew(check, claim, 45*time.Second)
				done()
				if err != nil {
					stop(fmt.Errorf("execution lease lost: %w", err))
					return
				}
			}
		}
	}()
	return execCtx, lease, nil
}
