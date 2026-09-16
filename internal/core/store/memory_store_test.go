package store

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
)

func TestRedisMemoryStore_GetMessages_EmptyMemoryReturnsEmptySlice(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	memoryStore := NewRedisMemoryStore(rdb, "empty", 20, time.Hour)

	messages, err := memoryStore.GetMessages(context.Background())
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected empty memory, got %d messages", len(messages))
	}
}

func TestRedisMemoryStore_AppendMessageThenGetMessages_RoundTripsListEntries(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	memoryStore := NewRedisMemoryStore(rdb, "roundtrip", 20, time.Hour)

	ctx := context.Background()
	if err := memoryStore.AppendMessage(ctx, schema.UserMessage("hello")); err != nil {
		t.Fatalf("AppendMessage user: %v", err)
	}
	if err := memoryStore.AppendMessage(ctx, schema.AssistantMessage("world", nil)); err != nil {
		t.Fatalf("AppendMessage assistant: %v", err)
	}

	messages, err := memoryStore.GetMessages(ctx)
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Content != "hello" || messages[1].Content != "world" {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}
