package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

// MemoryStore 对话记忆存储接口
type MemoryStore interface {
	GetMessages(ctx context.Context) ([]*schema.Message, error)
	AppendMessage(ctx context.Context, message *schema.Message) error
	ClearMessages(ctx context.Context) error
}

// RedisMemoryStore Redis实现的内存存储
type RedisMemoryStore struct {
	redisClient       *redis.Client
	memoryId          string
	maxMemoryMessages int
	ttl               time.Duration
}

func NewRedisMemoryStore(redisClient *redis.Client, memoryId string, maxMemoryMessages int, ttl time.Duration) *RedisMemoryStore {
	return &RedisMemoryStore{
		redisClient:       redisClient,
		memoryId:          memoryId,
		maxMemoryMessages: maxMemoryMessages,
		ttl:               ttl,
	}
}

func (r *RedisMemoryStore) GetMessages(ctx context.Context) ([]*schema.Message, error) {
	key := fmt.Sprintf("memory:%s", r.memoryId)
	data, err := r.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	return decodeMessagesFromJSON(data)
}

func (r *RedisMemoryStore) AppendMessage(ctx context.Context, message *schema.Message) error {
	key := fmt.Sprintf("memory:%s", r.memoryId)
	data, err := encodeMessagesToJSON(message)
	if err != nil {
		return err
	}
	pipe := r.redisClient.Pipeline()
	pipe.RPush(ctx, key, data)
	if r.maxMemoryMessages > 0 {
		pipe.LTrim(ctx, key, -int64(r.maxMemoryMessages), -1)
	}
	pipe.Expire(ctx, key, r.ttl)
	_, err = pipe.Exec(ctx)
	return err
}
func (r *RedisMemoryStore) ClearMessages(ctx context.Context) error {
	key := fmt.Sprintf("memory:%s", r.memoryId)
	return r.redisClient.Del(ctx, key).Err()
}

func encodeMessagesToJSON(msgs *schema.Message) ([]byte, error) {
	return json.Marshal(msgs)
}

func decodeMessagesFromJSON(data []byte) ([]*schema.Message, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var msgs []*schema.Message
	err := json.Unmarshal(data, &msgs)
	return msgs, err
}
