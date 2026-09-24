package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestMemoryStore(t *testing.T) (*RedisMemoryStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewRedisMemoryStore(client, 50, time.Hour), server
}

func TestRedisMemoryStoreMetadataRoundTrip(t *testing.T) {
	store, _ := newTestMemoryStore(t)
	ctx := context.Background()

	err := store.SetMetadata(ctx, "app-1", &MemoryMetadata{
		Summary:          "keep the user requirement",
		Round:            3,
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
		Summarizing:      true,
		SummaryError:     "temporary",
	})
	require.NoError(t, err)

	metadata, err := store.GetMetadata(ctx, "app-1")
	require.NoError(t, err)
	require.Equal(t, int64(3), metadata.Round)
	require.Equal(t, int64(30), metadata.TotalTokens)
	require.True(t, metadata.Summarizing)
	require.Equal(t, "keep the user requirement", metadata.Summary)
	require.Equal(t, "temporary", metadata.SummaryError)
}

func TestRedisMemoryStoreSummaryStateCanBeClearedAfterFailure(t *testing.T) {
	store, _ := newTestMemoryStore(t)
	ctx := context.Background()

	require.NoError(t, store.SetMetadata(ctx, "app-2", &MemoryMetadata{
		Summarizing:  true,
		SummaryError: "summary failed",
	}))
	metadata, err := store.GetMetadata(ctx, "app-2")
	require.NoError(t, err)
	metadata.Summarizing = false
	require.NoError(t, store.SetMetadata(ctx, "app-2", metadata))

	metadata, err = store.GetMetadata(ctx, "app-2")
	require.NoError(t, err)
	require.False(t, metadata.Summarizing)
	require.Equal(t, "summary failed", metadata.SummaryError)
}

func TestRedisMemoryStoreSummarySuccessPersistsSummaryAndUsage(t *testing.T) {
	store, _ := newTestMemoryStore(t)
	ctx := context.Background()

	require.NoError(t, store.SetMetadata(ctx, "app-success", &MemoryMetadata{
		Summarizing: true,
	}))
	metadata, err := store.GetMetadata(ctx, "app-success")
	require.NoError(t, err)
	metadata.Summary = "The user needs a Vue landing page."
	metadata.Round = 1
	metadata.Summarizing = false
	metadata.SummaryError = ""
	require.NoError(t, store.SetMetadata(ctx, "app-success", metadata))
	require.NoError(t, store.AddTokenUsage(ctx, "app-success", 12, 8))

	metadata, err = store.GetMetadata(ctx, "app-success")
	require.NoError(t, err)
	require.Equal(t, "The user needs a Vue landing page.", metadata.Summary)
	require.Equal(t, int64(1), metadata.Round)
	require.Equal(t, int64(20), metadata.TotalTokens)
	require.False(t, metadata.Summarizing)
}

func TestRedisMemoryStoreSummaryLockSerializesCallers(t *testing.T) {
	store, _ := newTestMemoryStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	unlock, err := store.Lock(ctx, "summary:app-3")
	require.NoError(t, err)
	defer unlock()

	contenderDone := make(chan error, 1)
	go func() {
		contenderCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, contenderErr := store.Lock(contenderCtx, "summary:app-3")
		contenderDone <- contenderErr
	}()

	require.Error(t, <-contenderDone)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		unlock()
	}()
	wg.Wait()
}
