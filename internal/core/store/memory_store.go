package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	GetSummary(ctx context.Context, memoryId string) (string, error)
	SetSummary(ctx context.Context, memoryId string, summary string) error
	GetRecentMessages(ctx context.Context, memoryId string) ([]*schema.Message, error)
	ClearMemory(ctx context.Context, memoryId string) error
	GetMetadata(ctx context.Context, memoryId string) (*MemoryMetadata, error)
	SetMetadata(ctx context.Context, memoryId string, metadata *MemoryMetadata) error
	AddTokenUsage(ctx context.Context, memoryId string, promptTokens, completionTokens int64) error
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
type MemoryMetadata struct {
	Summary          string
	Round            int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	Summarizing      bool
	SummaryError     string
	UpdatedAt        time.Time
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
	summary, err := r.GetSummary(ctx, memoryId)
	if err != nil {
		return nil, err
	}
	recentMessages, err := r.GetRecentMessages(ctx, memoryId)
	if err != nil {
		return nil, err
	}
	messages := make([]*schema.Message, 0, len(recentMessages)+1)
	if summary != "" {
		messages = append(messages, schema.SystemMessage(summary))
	}
	messages = append(messages, recentMessages...)
	return messages, nil
}

func (r *RedisMemoryStore) GetSummary(ctx context.Context, memoryId string) (string, error) {
	metadata, err := r.GetMetadata(ctx, memoryId)
	if err != nil {
		return "", err
	}
	return metadata.Summary, nil
}

func (r *RedisMemoryStore) SetSummary(ctx context.Context, memoryId string, summary string) error {
	metadata, err := r.GetMetadata(ctx, memoryId)
	if err != nil {
		return err
	}
	metadata.Summary = summary
	return r.SetMetadata(ctx, memoryId, metadata)
}

func (r *RedisMemoryStore) GetRecentMessages(ctx context.Context, memoryId string) ([]*schema.Message, error) {
	items, err := r.redisClient.LRange(ctx, memoryMessagesKey(memoryId), 0, -1).Result()
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
	data, err := encodeMessagesToJSON(message)
	if err != nil {
		return err
	}
	key := memoryMessagesKey(memoryId)
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
	return r.redisClient.Del(ctx, memoryMessagesKey(memoryId)).Err()
}

func (r *RedisMemoryStore) ClearMemory(ctx context.Context, memoryId string) error {
	return r.redisClient.Del(ctx, memoryMetadataKey(memoryId), memoryMessagesKey(memoryId), memorySummaryKey(memoryId)).Err()
}

func (r *RedisMemoryStore) GetMetadata(ctx context.Context, memoryId string) (*MemoryMetadata, error) {
	values, err := r.redisClient.HGetAll(ctx, memoryMetadataKey(memoryId)).Result()
	if err != nil {
		return nil, err
	}
	metadata := &MemoryMetadata{}
	metadata.Summary = values["summary"]
	metadata.SummaryError = values["summary_error"]
	metadata.Summarizing = values["summarizing"] == "1"
	metadata.Round = parseInt64(values["round"])
	metadata.PromptTokens = parseInt64(values["prompt_tokens"])
	metadata.CompletionTokens = parseInt64(values["completion_tokens"])
	metadata.TotalTokens = parseInt64(values["total_tokens"])
	if updatedAt := parseInt64(values["updated_at"]); updatedAt > 0 {
		metadata.UpdatedAt = time.Unix(updatedAt, 0)
	}
	return metadata, nil
}

func (r *RedisMemoryStore) SetMetadata(ctx context.Context, memoryId string, metadata *MemoryMetadata) error {
	if metadata == nil {
		return r.redisClient.Del(ctx, memoryMetadataKey(memoryId)).Err()
	}
	updatedAt := metadata.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	values := map[string]interface{}{
		"summary":           metadata.Summary,
		"round":             metadata.Round,
		"prompt_tokens":     metadata.PromptTokens,
		"completion_tokens": metadata.CompletionTokens,
		"total_tokens":      metadata.TotalTokens,
		"summarizing":       boolToInt(metadata.Summarizing),
		"summary_error":     metadata.SummaryError,
		"updated_at":        updatedAt.Unix(),
	}
	pipe := r.redisClient.Pipeline()
	pipe.HSet(ctx, memoryMetadataKey(memoryId), values)
	pipe.Expire(ctx, memoryMetadataKey(memoryId), r.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisMemoryStore) AddTokenUsage(ctx context.Context, memoryId string, promptTokens, completionTokens int64) error {
	key := memoryMetadataKey(memoryId)
	pipe := r.redisClient.Pipeline()
	pipe.HIncrBy(ctx, key, "prompt_tokens", promptTokens)
	pipe.HIncrBy(ctx, key, "completion_tokens", completionTokens)
	pipe.HIncrBy(ctx, key, "total_tokens", promptTokens+completionTokens)
	pipe.HSet(ctx, key, "updated_at", time.Now().Unix())
	pipe.Expire(ctx, key, r.ttl)
	_, err := pipe.Exec(ctx)
	return err
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

func memorySummaryKey(memoryId string) string {
	return fmt.Sprintf("memory:%s:summary", memoryId)
}

func memoryMetadataKey(memoryId string) string {
	return fmt.Sprintf("memory:%s:meta", memoryId)
}

func memoryMessagesKey(memoryId string) string {
	return fmt.Sprintf("memory:%s:messages", memoryId)
}

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

func parseInt64(value string) int64 {
	if value == "" {
		return 0
	}
	result, _ := strconv.ParseInt(value, 10, 64)
	return result
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
