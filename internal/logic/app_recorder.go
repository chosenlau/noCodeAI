package logic

import (
	"context"
	"strconv"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/dal/query"
	"github.com/cloudwego/eino/schema"
)

type appTokenUsageRecorder struct {
	service *AppService
	appID   int64
}

func (r *appTokenUsageRecorder) RecordTokenUsage(ctx context.Context, usage *schema.TokenUsage) {
	if r == nil || r.service == nil || usage == nil {
		return
	}
	promptTokens := int64(usage.PromptTokens)
	completionTokens := int64(usage.CompletionTokens)
	totalTokens := promptTokens + completionTokens
	if totalTokens == 0 {
		return
	}

	q := query.Use(r.service.db)
	if _, err := q.App.WithContext(ctx).
		Where(q.App.ID.Eq(r.appID), q.App.IsDelete.Eq(0)).
		UpdateSimple(
			q.App.PromptTokens.Add(promptTokens),
			q.App.CompletionTokens.Add(completionTokens),
			q.App.TokenUsage.Add(totalTokens),
		); err != nil {
		logger.Errorf("update app token usage failed: %v", err)
	}
	if r.service.memoryStore != nil {
		if err := r.service.memoryStore.AddTokenUsage(
			ctx,
			strconv.FormatInt(r.appID, 10),
			promptTokens,
			completionTokens,
		); err != nil {
			logger.Errorf("update memory token usage failed: %v", err)
		}
	}
}
