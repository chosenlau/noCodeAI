package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSummaryCleanupDoesNotClearReplacementOwnerOrUsage(t *testing.T) {
	s, _ := newTestMemoryStore(t)
	ctx := context.Background()
	key := memoryMetadataKey("summary-test")
	require.NoError(t, s.redisClient.HSet(ctx, key, "summary_owner", "new", "summarizing", "1", "summary", "keep", "total_tokens", "123").Err())
	require.NoError(t, s.ClearSummaryOwner(ctx, "summary-test", "old"))
	require.Equal(t, "1", s.redisClient.HGet(ctx, key, "summarizing").Val())
	require.NoError(t, s.ClearSummaryOwner(ctx, "summary-test", "new"))
	require.Equal(t, "0", s.redisClient.HGet(ctx, key, "summarizing").Val())
	require.Equal(t, "keep", s.redisClient.HGet(ctx, key, "summary").Val())
	require.Equal(t, "123", s.redisClient.HGet(ctx, key, "total_tokens").Val())
	require.False(t, s.redisClient.HExists(ctx, key, "summary_owner").Val())
}
