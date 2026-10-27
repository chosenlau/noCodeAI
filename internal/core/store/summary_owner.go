package store

import "context"

// Owner comparison prevents a delayed cleanup from clearing a new summarizer.
func (r *RedisMemoryStore) ClearSummaryOwner(ctx context.Context, id, owner string) error {
	return r.redisClient.Eval(ctx, `if redis.call('HGET',KEYS[1],'summary_owner') == ARGV[1] then redis.call('HSET',KEYS[1],'summarizing','0'); redis.call('HDEL',KEYS[1],'summary_owner'); return 1 end return 0`, []string{memoryMetadataKey(id)}, owner).Err()
}
