package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

// MemoryStore 对话记忆存储接口
type MemoryStore interface {
	GetMessages(ctx context.Context, memoryId string) ([]*schema.Message, error)
	AppendMessage(ctx context.Context, message *schema.Message, memoryId string) error
	ClearMessages(ctx context.Context, memoryId string) error
	AddAssistantMessage(ctx context.Context, assistantMsg string, memoryId string) error
	AddUserMessage(ctx context.Context, userMsg string, memoryId string) error
}

type HistoryCache interface {
	GetHistory(ctx context.Context, appID string) ([]*model.ChatHistory, error)
	SetHistory(ctx context.Context, appID string, records []*model.ChatHistory) error
	ClearHistory(ctx context.Context, appID string) error
}

// RedisMemoryStore Redis实现的内存存储
type RedisMemoryStore struct {
	redisClient       *redis.Client
	maxMemoryMessages int
	ttl               time.Duration
}

func NewRedisMemoryStore(redisClient *redis.Client, maxMemoryMessages int, ttl time.Duration) *RedisMemoryStore {
	return &RedisMemoryStore{
		redisClient:       redisClient,
		maxMemoryMessages: maxMemoryMessages,
		ttl:               ttl,
	}
}

func (r *RedisMemoryStore) AddAssistantMessage(ctx context.Context, assistantMsg string, memoryId string) error {
	message := schema.AssistantMessage(assistantMsg, nil)
	return r.AppendMessage(ctx, message, memoryId)
}

func (r *RedisMemoryStore) AddUserMessage(ctx context.Context, userMsg string, memoryId string) error {
	message := schema.UserMessage(userMsg)
	return r.AppendMessage(ctx, message, memoryId)
}

func (r *RedisMemoryStore) GetMessages(ctx context.Context, memoryId string) ([]*schema.Message, error) {
	key := fmt.Sprintf("memory:%s", memoryId)
	items, err := r.redisClient.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []*schema.Message{}, nil
	}

	messages := make([]*schema.Message, 0, len(items))
	for _, item := range items {
		msg, err := decodeMessageFromJSON([]byte(item))
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (r *RedisMemoryStore) AppendMessage(ctx context.Context, message *schema.Message, memoryId string) error {
	key := fmt.Sprintf("memory:%s", memoryId)
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
func (r *RedisMemoryStore) ClearMessages(ctx context.Context, memoryId string) error {
	key := fmt.Sprintf("memory:%s", memoryId)
	return r.redisClient.Del(ctx, key).Err()
}

func (r *RedisMemoryStore) GetHistory(ctx context.Context, appID string) ([]*model.ChatHistory, error) {
	data, err := r.redisClient.Get(ctx, "history:"+appID).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []*model.ChatHistory
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *RedisMemoryStore) SetHistory(ctx context.Context, appID string, records []*model.ChatHistory) error {
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	return r.redisClient.Set(ctx, "history:"+appID, data, 24*time.Hour).Err()
}

func (r *RedisMemoryStore) ClearHistory(ctx context.Context, appID string) error {
	return r.redisClient.Del(ctx, "history:"+appID).Err()
}

var _ HistoryCache = (*RedisMemoryStore)(nil)

func encodeMessagesToJSON(msgs *schema.Message) ([]byte, error) {
	return json.Marshal(msgs)
}

func decodeMessageFromJSON(data []byte) (*schema.Message, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var msg schema.Message
	err := json.Unmarshal(data, &msg)
	return &msg, err
}
