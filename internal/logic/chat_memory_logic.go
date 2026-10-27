package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/dal/query"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

func historyMessage(r *model.ChatHistory) *schema.Message {
	if r.MessageType == string(enum.UserMessageType) {
		return schema.UserMessage(r.Message)
	}
	return schema.AssistantMessage(r.Message, nil)
}

// Summary.TurnNumber is the last fully covered conversation turn. New messages
// remain recoverable regardless of Redis eviction and summary creation time.
func (s *ChatHistoryService) loadUncoveredConversation(ctx context.Context, appID int64) (*model.ChatHistory, []*model.ChatHistory, error) {
	q := query.Use(s.db).ChatHistory
	previous, err := q.WithContext(ctx).Where(q.AppID.Eq(appID), q.IsDelete.Eq(0), q.MessageType.Eq(string(enum.SummaryMessageType))).Order(q.CreateTime.Desc(), q.ID.Desc()).First()
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	recent := q.WithContext(ctx).Where(q.AppID.Eq(appID), q.IsDelete.Eq(0), q.MessageType.Neq(string(enum.SummaryMessageType)))
	if previous != nil {
		recent = recent.Where(q.TurnNumber.Gt(previous.TurnNumber))
	}
	records, err := recent.Order(q.CreateTime.Asc(), q.ID.Asc()).Limit(1001).Find()
	if len(records) > 1000 {
		return nil, nil, fmt.Errorf("conversation requires summary before continuing")
	}
	return previous, records, err
}

func needsConversationSummary(records []*model.ChatHistory) bool {
	if len(records) < 4 {
		return false
	}
	return len(records) >= 16 || contextmgr.Estimate(records) >= 12000
}

// Retain complete turns even when the requested record boundary falls between
// a user's request and its answer. A coverage watermark never splits a turn.
func conversationSummaryCutoff(records []*model.ChatHistory, keep int) (int32, bool) {
	if keep < 1 || len(records) <= keep {
		return 0, false
	}
	firstRetainedTurn := records[len(records)-keep].TurnNumber
	var cutoff int32
	found := false
	for _, r := range records[:len(records)-keep] {
		if r.TurnNumber < firstRetainedTurn && (!found || r.TurnNumber > cutoff) {
			cutoff, found = r.TurnNumber, true
		}
	}
	return cutoff, found
}

// Bounded batches recover early requirements from SQL without overflowing the
// summary model. All calls share the summary execution lease and budget.
func (s *ChatHistoryService) summarizeConversationBatches(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	limit := s.summaryAgent.ContextInputLimit() - 2048
	var previous *schema.Message
	var usage schema.TokenUsage
	for len(messages) > 0 {
		batch := []*schema.Message{}
		if previous != nil {
			batch = append(batch, schema.SystemMessage(previous.Content))
		}
		before := len(messages)
		for len(messages) > 0 && contextmgr.Estimate(append(batch, messages[0])) < limit {
			batch = append(batch, messages[0])
			messages = messages[1:]
		}
		if len(messages) == before {
			return nil, contextmgr.ErrTooLarge
		}
		result, err := s.summaryAgent.SummarizeChat(ctx, batch)
		if err != nil {
			return nil, err
		}
		if result == nil || strings.TrimSpace(result.Content) == "" || len(result.Content) > 6000 {
			return nil, errors.New("invalid conversation summary")
		}
		if result.ResponseMeta != nil && result.ResponseMeta.Usage != nil {
			u := result.ResponseMeta.Usage
			usage.PromptTokens += u.PromptTokens
			usage.CompletionTokens += u.CompletionTokens
			usage.TotalTokens += u.TotalTokens
		}
		previous = result
	}
	if previous != nil {
		previous.ResponseMeta = &schema.ResponseMeta{Usage: &usage}
	}
	return previous, nil
}
