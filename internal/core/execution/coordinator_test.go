package execution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type coordinatorLeaseSource struct {
	acquireErr error
	key        string
	ctx        context.Context
	cancel     context.CancelCauseFunc
	releases   atomic.Int32
}

func (s *coordinatorLeaseSource) AcquireLease(ctx context.Context, key string) (context.Context, func(), error) {
	s.key = key
	s.ctx = ctx
	if s.acquireErr != nil {
		return nil, nil, s.acquireErr
	}
	leaseCtx, cancel := context.WithCancelCause(ctx)
	s.cancel = cancel
	return leaseCtx, func() { s.releases.Add(1); cancel(context.Canceled) }, nil
}

type coordinatorPublicationStore struct {
	claim             Claim
	claimErr          error
	claims            atomic.Int32
	releases          atomic.Int32
	releaseErr        error
	releaseContextErr error
	releasedClaim     Claim
	appID             int64
	owner             string
	ttl               time.Duration
}

func (s *coordinatorPublicationStore) Claim(_ context.Context, appID int64, owner string, ttl time.Duration) (Claim, error) {
	s.claims.Add(1)
	s.appID, s.owner, s.ttl = appID, owner, ttl
	return s.claim, s.claimErr
}
func (*coordinatorPublicationStore) Renew(context.Context, Claim, time.Duration) error { return nil }
func (s *coordinatorPublicationStore) Release(ctx context.Context, claim Claim) error {
	s.releaseContextErr = ctx.Err()
	s.releasedClaim = claim
	s.releases.Add(1)
	return s.releaseErr
}
func (*coordinatorPublicationStore) Current(context.Context, int64) (Version, error) {
	return Version{}, errors.New("unexpected Current")
}
func (*coordinatorPublicationStore) Publish(context.Context, Publication, map[string]string, string) (int64, error) {
	return 0, errors.New("unexpected Publish")
}

func TestCoordinatorClaimFailureReleasesRedis(t *testing.T) {
	claimErr := errors.New("SQL claim rejected")
	locks := &coordinatorLeaseSource{}
	persistence := &coordinatorPublicationStore{claimErr: claimErr}
	ctx, lease, err := Acquire(context.Background(), locks, persistence, 42, "run-7", NewBudget(BudgetOptions{}))
	if !errors.Is(err, claimErr) || ctx != nil || lease != nil {
		t.Fatalf("failed claim must not expose execution: ctx=%v lease=%v err=%v", ctx, lease, err)
	}
	if locks.releases.Load() != 1 || persistence.releases.Load() != 0 {
		t.Fatalf("wrong rollback: redis=%d SQL=%d", locks.releases.Load(), persistence.releases.Load())
	}
	if locks.ctx.Err() != context.Canceled {
		t.Fatal("failed Acquire retained execution context")
	}
	if locks.key != "42" || persistence.appID != 42 || persistence.owner != "run-7" || persistence.ttl <= 0 {
		t.Fatal("claim did not use project identity and bounded lease")
	}
}

func TestCoordinatorRedisFailureNeverClaimsSQL(t *testing.T) {
	lockErr := errors.New("Redis ownership unavailable")
	locks := &coordinatorLeaseSource{acquireErr: lockErr}
	persistence := &coordinatorPublicationStore{}
	ctx, lease, err := Acquire(context.Background(), locks, persistence, 42, "run-7", NewBudget(BudgetOptions{}))
	if !errors.Is(err, lockErr) || ctx != nil || lease != nil {
		t.Fatalf("unexpected Acquire result: %v", err)
	}
	if persistence.claims.Load() != 0 || locks.releases.Load() != 0 {
		t.Fatal("failed Redis acquisition must not claim SQL or release another owner's lock")
	}
	if locks.ctx.Err() != context.Canceled {
		t.Fatal("failed Acquire retained execution context")
	}
}

func TestCoordinatorReleaseIsConcurrentIdempotentAndIndependentOfParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	claim := Claim{AppID: 42, Owner: "run-7", Epoch: 3, BaseVersionID: 2, BasePath: "version-2"}
	locks := &coordinatorLeaseSource{}
	// Redis release must still happen if SQL release fails.
	persistence := &coordinatorPublicationStore{claim: claim, releaseErr: errors.New("SQL cleanup unavailable")}
	ctx, lease, err := Acquire(parent, locks, persistence, 42, "run-7", NewBudget(BudgetOptions{}))
	if err != nil {
		t.Fatal(err)
	}
	if lease.Claim != claim {
		t.Fatalf("publication identity lost: %+v", lease.Claim)
	}
	cancel()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(lease.Release)
	}
	wg.Wait()
	if persistence.releases.Load() != 1 || locks.releases.Load() != 1 {
		t.Fatalf("Release must be once: SQL=%d Redis=%d", persistence.releases.Load(), locks.releases.Load())
	}
	if persistence.releaseContextErr != nil || persistence.releasedClaim != claim {
		t.Fatalf("cleanup must use live context and original claim: %v %+v", persistence.releaseContextErr, persistence.releasedClaim)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("Release left execution alive: %v", ctx.Err())
	}
}

func TestCoordinatorPropagatesParentAndLeaseCancellation(t *testing.T) {
	for _, source := range []string{"parent", "redis-lease", "budget-deadline"} {
		t.Run(source, func(t *testing.T) {
			parent, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			opts := BudgetOptions{}
			if source == "budget-deadline" {
				opts.Timeout = 20 * time.Millisecond
			}
			budget := NewBudget(opts)
			locks := &coordinatorLeaseSource{}
			persistence := &coordinatorPublicationStore{claim: Claim{AppID: 42, Owner: "run-7", Epoch: 3}}
			ctx, lease, err := Acquire(parent, locks, persistence, 42, "run-7", budget)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Release()
			if GetBudget(ctx) != budget {
				t.Fatal("execution lost shared budget")
			}
			cause := errors.New("ownership lost")
			switch source {
			case "parent":
				cancel(cause)
			case "redis-lease":
				locks.cancel(cause)
			case "budget-deadline":
				cause = context.DeadlineExceeded
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("execution continued after cancellation")
			}
			if !errors.Is(context.Cause(ctx), cause) {
				t.Fatalf("cancellation cause lost: %v", context.Cause(ctx))
			}
		})
	}
}
