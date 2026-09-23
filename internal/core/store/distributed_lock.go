package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type DistributedLocker interface {
	Lock(ctx context.Context, key string) (func(), error)
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
