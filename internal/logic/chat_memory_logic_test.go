package logic

import (
	"strings"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/stretchr/testify/require"
)

func TestSummaryTriggerUsesUncoveredHistoryRatherThanLifetimeTurn(t *testing.T) {
	records := make([]*model.ChatHistory, 16)
	for i := range records {
		records[i] = &model.ChatHistory{TurnNumber: int32(100 + i/2), Message: "hello"}
	}
	require.True(t, needsConversationSummary(records))
	// Persisting a coverage watermark leaves only recent turns uncovered.
	require.False(t, needsConversationSummary(records[4:]))
	require.False(t, needsConversationSummary(records[:2]))
	for _, r := range records[:4] {
		r.Message = strings.Repeat("x", 4000)
	}
	require.True(t, needsConversationSummary(records[:4]), "large conversations trigger before eight turns")
}

func TestSummaryCutoffRetainsWholeTurnAtRecordBoundary(t *testing.T) {
	var records []*model.ChatHistory
	for _, turn := range []int32{1, 1, 2, 2, 3, 3, 4} {
		records = append(records, &model.ChatHistory{TurnNumber: turn})
	}
	cutoff, ok := conversationSummaryCutoff(records, 2)
	require.True(t, ok)
	require.EqualValues(t, 2, cutoff, "turn three must remain whole with the newest user request")
	_, ok = conversationSummaryCutoff(records[:2], 1)
	require.False(t, ok, "a single turn cannot be partially summarized")
}
