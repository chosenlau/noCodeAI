package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAcquireLeaseReturnsBusyWithoutWaitingForHolder(t *testing.T) {
	store, _ := newTestMemoryStore(t)
	_, release, err := store.AcquireLease(context.Background(), "app:busy")
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, cleanup, acquireErr := store.AcquireLease(ctx, "app:busy")
		if cleanup != nil {
			cleanup()
		}
		result <- acquireErr
	}()
	select {
	case err := <-result:
		require.ErrorIs(t, err, ErrApplicationBusy)
	case <-time.After(time.Second):
		t.Fatal("busy acquisition waited for the holder instead of returning immediately")
	}
}

func TestLeaseReleaseDoesNotDeleteReplacementOwner(t *testing.T) {
	store, server := newTestMemoryStore(t)
	leaseCtx, release, err := store.AcquireLease(context.Background(), "app:replacement")
	require.NoError(t, err)
	defer release()
	key := "codegen:lock:app:replacement"
	server.FastForward(time.Minute)
	require.NoError(t, store.redisClient.Set(context.Background(), key, "new-owner", time.Minute).Err())
	release()
	require.ErrorIs(t, leaseCtx.Err(), context.Canceled)
	owner, err := server.Get(key)
	require.NoError(t, err)
	require.Equal(t, "new-owner", owner)
	// Release is idempotent even when ownership has already changed.
	release()
	owner, err = server.Get(key)
	require.NoError(t, err)
	require.Equal(t, "new-owner", owner)
}

func TestRenewLeaseScriptRequiresMatchingOwner(t *testing.T) {
	store, server := newTestMemoryStore(t)
	ctx := context.Background()
	key := "codegen:lock:app:renew"
	require.NoError(t, store.redisClient.Set(ctx, key, "owner", 45*time.Second).Err())
	server.FastForward(30 * time.Second)
	require.Equal(t, 15*time.Second, server.TTL(key))
	owned, err := store.redisClient.Eval(ctx, renewLockScript, []string{key}, "owner", int64(45000)).Int()
	require.NoError(t, err)
	require.Equal(t, 1, owned)
	require.Equal(t, 45*time.Second, server.TTL(key))
	server.FastForward(10 * time.Second)
	owned, err = store.redisClient.Eval(ctx, renewLockScript, []string{key}, "stale-owner", int64(45000)).Int()
	require.NoError(t, err)
	require.Zero(t, owned)
	require.Equal(t, 35*time.Second, server.TTL(key))
	owner, err := server.Get(key)
	require.NoError(t, err)
	require.Equal(t, "owner", owner)
	server.FastForward(time.Minute)
	owned, err = store.redisClient.Eval(ctx, renewLockScript, []string{key}, "owner", int64(45000)).Int()
	require.NoError(t, err)
	require.Zero(t, owned)
	require.False(t, server.Exists(key), "renewal must not recreate an expired lease")
}

func TestLeaseFollowsParentCancellationAndReleasesOwnKey(t *testing.T) {
	store, server := newTestMemoryStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	leaseCtx, release, err := store.AcquireLease(ctx, "app:cancel")
	require.NoError(t, err)
	defer release()
	cancel()
	select {
	case <-leaseCtx.Done():
		require.ErrorIs(t, context.Cause(leaseCtx), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("lease context did not inherit parent cancellation")
	}
	release()
	require.False(t, server.Exists("codegen:lock:app:cancel"))
	_, cleanup, err := store.AcquireLease(ctx, "app:cancelled-before-acquire")
	if cleanup != nil {
		cleanup()
	}
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, server.Exists("codegen:lock:app:cancelled-before-acquire"))
}
