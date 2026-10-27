package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

type DistributedLocker interface {
	Lock(ctx context.Context, key string) (func(), error)
}

// LeaseLocker exposes cancellation on lease loss. All app writers share one key.
type LeaseLocker interface {
	AcquireLease(context.Context, string) (context.Context, func(), error)
}

var ErrApplicationBusy = errors.New("application execution is busy")

const renewLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
 return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`

func (r *RedisMemoryStore) AcquireLease(ctx context.Context, key string) (context.Context, func(), error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, nil, err
	}
	token := hex.EncodeToString(b)
	lockKey := "codegen:lock:" + key
	const ttl = 45 * time.Second
	ok, err := r.redisClient.SetNX(ctx, lockKey, token, ttl).Result()
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrApplicationBusy
	}
	leaseCtx, cancel := context.WithCancelCause(ctx)
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel(context.Canceled)
			cleanup, done := context.WithTimeout(context.Background(), 3*time.Second)
			defer done()
			_ = r.redisClient.Eval(cleanup, releaseLockScript, []string{lockKey}, token).Err()
		})
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				check, done := context.WithTimeout(leaseCtx, 3*time.Second)
				owned, err := r.redisClient.Eval(check, renewLockScript, []string{lockKey}, token, ttl.Milliseconds()).Int()
				done()
				if err != nil || owned != 1 {
					cancel(fmt.Errorf("application lease lost: %v", err))
					return
				}
			}
		}
	}()
	return leaseCtx, release, nil
}

const releaseLockScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`

func (r *RedisMemoryStore) Lock(ctx context.Context, key string) (func(), error) {
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate lock token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	lockKey := "codegen:lock:" + key
	ttl := 30 * time.Minute

	for {
		ok, err := r.redisClient.SetNX(ctx, lockKey, token, ttl).Result()
		if err != nil {
			return nil, err
		}
		if ok {
			var once sync.Once
			return func() {
				once.Do(func() {
					_ = r.redisClient.Eval(
						context.Background(),
						releaseLockScript,
						[]string{lockKey},
						token,
					).Err()
				})
			}, nil
		}

		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

var _ DistributedLocker = (*RedisMemoryStore)(nil)
