package logic

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/dal/query"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gen/field"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChatHistoryService struct {
	db           *gorm.DB
	memoryStore  store.MemoryStore
	summaryAgent *agent.ChatSummaryAgent
}

func NewChatHistoryService(db *gorm.DB, memoryStore store.MemoryStore, summaryAgent *agent.ChatSummaryAgent) *ChatHistoryService {
	if db == nil {
		panic("chat history service requires a non-nil database")
	}
	if memoryStore == nil {
		panic("chat history service requires a non-nil memory store")
	}
	return &ChatHistoryService{
		db: db, memoryStore: memoryStore, summaryAgent: summaryAgent,
	}
}

func (s *ChatHistoryService) ListAppChatHistoryByCursor(
	ctx context.Context,
	appId int64,
	pageSize int32,
	lastCreateTime time.Time,
	lastID int64,
	loginUser *api.UserVo,
) (*api.CursorResponse, error) {
	if appId <= 0 || pageSize <= 0 || pageSize > 50 {
		return nil, errorutil.ParamsError
	}
	if loginUser == nil {
		return nil, errorutil.NotLoginError
	}

	q := query.Use(s.db)
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(appId), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return nil, err
	}
	if app.UserID != loginUser.ID && loginUser.UserRole != string(enum.RoleAdmin) {
		return nil, errorutil.NotAuthError
	}

	var records []*model.ChatHistory
	cache, ok := s.memoryStore.(store.HistoryCache)
	if ok {
		records, err = cache.GetHistory(ctx, strconv.FormatInt(appId, 10))
		if err != nil {
			return nil, err
		}
	}
	if records == nil {
		records, err = q.ChatHistory.WithContext(ctx).
			Where(q.ChatHistory.AppID.Eq(appId), q.ChatHistory.IsDelete.Eq(0)).
			Where(q.ChatHistory.MessageType.Neq(string(enum.SummaryMessageType))).
			Order(q.ChatHistory.CreateTime.Desc(), q.ChatHistory.ID.Desc()).
			Limit(100).Find()
		if err != nil {
			return nil, err
		}
		if ok {
			_ = cache.SetHistory(ctx, strconv.FormatInt(appId, 10), records)
		}
	}

	filtered := make([]*model.ChatHistory, 0, len(records))
	for _, record := range records {
		if !lastCreateTime.IsZero() &&
			(record.CreateTime.After(lastCreateTime) ||
				(record.CreateTime.Equal(lastCreateTime) && record.ID >= lastID)) {
			continue
		}
		filtered = append(filtered, record)
		if len(filtered) > int(pageSize) {
			break
		}
	}
	hasMore := len(filtered) > int(pageSize)
	if hasMore {
		filtered = filtered[:pageSize]
	}
	result := &api.CursorResponse{Records: filtered, HasMore: hasMore}
	if len(filtered) > 0 {
		last := filtered[len(filtered)-1]
		result.NextCreateTime = last.CreateTime
		result.NextId = last.ID
	}
	return result, nil
}

func (s *ChatHistoryService) ListAppChatHistoryByPage(ctx context.Context,
	appId int64, pageSize int32, lastCreateTime time.Time, lastID int64, loginUser *api.UserVo) (*response.PageResponse[*model.ChatHistory], error) {

	// Validate basic parameters.
	if appId == 0 || appId < 0 || pageSize <= 0 || pageSize > 50 {
		return nil, errorutil.ParamsError
	}
	if loginUser == nil {
		return nil, errorutil.NotLoginError
	}

	// Create query handle.
	q := query.Use(s.db)
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(appId), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return nil, err
	}
	if app.UserID != loginUser.ID && loginUser.UserRole != string(enum.RoleAdmin) {
		return nil, errorutil.NotAuthError
	}

	// Build query conditions.
	chatHistoryQuery := q.ChatHistory.WithContext(ctx).
		Where(q.ChatHistory.AppID.Eq(appId), q.ChatHistory.IsDelete.Eq(0)).
		Where(q.ChatHistory.MessageType.Neq(string(enum.SummaryMessageType)))

	// Apply cursor time filter.
	if !lastCreateTime.IsZero() {
		cursorCond := q.ChatHistory.CreateTime.Lt(lastCreateTime)
		if lastID > 0 {
			cursorCond = field.Or(
				q.ChatHistory.CreateTime.Lt(lastCreateTime),
				field.And(q.ChatHistory.CreateTime.Eq(lastCreateTime), q.ChatHistory.ID.Lt(lastID)),
			)
		}
		chatHistoryQuery = chatHistoryQuery.Where(cursorCond)
	}

	// Count total records.
	totalRow, err := chatHistoryQuery.Count()
	if err != nil {
		return nil, err
	}

	// Calculate total pages.
	totalPage := 0
	if totalRow > 0 {
		totalPage = int((totalRow + int64(pageSize) - 1) / int64(pageSize))
	}
	// TODO: cursor pagination.
	// Query chat history.
	chatHistoryList, err := chatHistoryQuery.
		Order(q.ChatHistory.CreateTime.Desc(), q.ChatHistory.ID.Desc()).
		Limit(int(pageSize)).
		Find()
	if err != nil {
		return nil, err
	}

	// Build page response.
	return &response.PageResponse[*model.ChatHistory]{
		Records:            chatHistoryList,
		PageNum:            1,
		PageSize:           int(pageSize),
		TotalPage:          totalPage,
		TotalRow:           int(totalRow),
		OptimizeCountQuery: true,
	}, nil
}

func (s *ChatHistoryService) DeleteByAppId(ctx context.Context, appId int64) error {
	// Validate app ID.
	if appId == 0 || appId < 0 {
		return errorutil.ParamsError.WithMessage("app ID is required")
	}

	// Create query handle.
	q := query.Use(s.db)
	_, err := q.ChatHistory.WithContext(ctx).
		Where(q.ChatHistory.AppID.Eq(appId), q.ChatHistory.IsDelete.Eq(0)).
		Update(q.ChatHistory.IsDelete, 1)
	if err != nil {
		return err
	}
	if cache, ok := s.memoryStore.(store.HistoryCache); ok {
		_ = cache.ClearHistory(ctx, strconv.FormatInt(appId, 10))
	}

	return nil
}

func (s *ChatHistoryService) AddChatMessage(ctx context.Context, appId int64,
	message string, messageType enum.ChatHistoryMessageTypeEnum, userId int64) error {

	// Validate parameters.
	if appId <= 0 || messageType == "" || userId <= 0 || message == "" {
		return errorutil.ParamsError
	}

	var turnNumber int32
	err := query.Use(s.db).Transaction(func(tx *query.Query) error {
		_, err := tx.App.WithContext(ctx).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(tx.App.ID.Eq(appId), tx.App.IsDelete.Eq(0)).
			First()
		if err != nil {
			return err
		}

		lastMessage, err := tx.ChatHistory.WithContext(ctx).
			Where(tx.ChatHistory.AppID.Eq(appId), tx.ChatHistory.IsDelete.Eq(0)).
			Order(tx.ChatHistory.CreateTime.Desc(), tx.ChatHistory.ID.Desc()).
			First()
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				turnNumber = 0
			} else {
				return err
			}
		} else {
			turnNumber = lastMessage.TurnNumber
		}

		// Start a new turn when current message is from user.
		if messageType == enum.UserMessageType {
			turnNumber += 1
		}

		// Generate message ID.
		chatMessageId, err := snowflake.GenerateSnowFlakeId()
		if err != nil {
			return err
		}

		// Create chat history record.
		return tx.WithContext(ctx).ChatHistory.Create(&model.ChatHistory{
			ID:          chatMessageId,
			AppID:       appId,
			Message:     message,
			MessageType: string(messageType),
			UserID:      userId,
			TurnNumber:  turnNumber,
		})
	})
	if err != nil {
		return err
	}
	if cache, ok := s.memoryStore.(store.HistoryCache); ok {
		_ = cache.ClearHistory(ctx, strconv.FormatInt(appId, 10))
	}

	return nil
}

func (s *ChatHistoryService) MaybeGenerateSummary(ctx context.Context, appId int64, userId int64) {
	if s.db == nil || s.memoryStore == nil || s.summaryAgent == nil {
		return
	}
	q := query.Use(s.db)
	lastMessage, err := q.ChatHistory.WithContext(ctx).
		Where(
			q.ChatHistory.AppID.Eq(appId),
			q.ChatHistory.IsDelete.Eq(0),
			q.ChatHistory.MessageType.Eq(string(enum.AIMessageType)),
		).
		Order(q.ChatHistory.CreateTime.Desc(), q.ChatHistory.ID.Desc()).
		First()
	if err != nil || lastMessage.TurnNumber < 20 {
		return
	}
	go s.generateSummary(context.Background(), appId, userId)
}

func (s *ChatHistoryService) IsSummarizing(ctx context.Context, appId int64) (bool, error) {
	if s.memoryStore == nil {
		return false, nil
	}
	metadata, err := s.memoryStore.GetMetadata(ctx, strconv.FormatInt(appId, 10))
	if err != nil {
		return false, err
	}
	return metadata.Summarizing, nil
}

// generateSummary creates a compact conversation summary.
func (s *ChatHistoryService) generateSummary(ctx context.Context, appId int64, userId int64) {
	locker, ok := s.memoryStore.(store.DistributedLocker)
	if ok {
		unlock, err := locker.Lock(ctx, "summary:"+strconv.FormatInt(appId, 10))
		if err != nil {
			logger.Infof("summary already in progress for app %d", appId)
			return
		}
		defer unlock()
	}

	if s.summaryAgent == nil {
		logger.Warn("summary agent is not initialized")
		return
	}

	if s.memoryStore == nil {
		logger.Warn("memory store is not initialized")
		return
	}

	memoryID := strconv.FormatInt(appId, 10)
	metadata, err := s.memoryStore.GetMetadata(ctx, memoryID)
	if err != nil {
		logger.Errorf("load memory metadata failed: %v", err)
		return
	}
	metadata.Summarizing = true
	metadata.SummaryError = ""
	if err := s.memoryStore.SetMetadata(ctx, memoryID, metadata); err != nil {
		logger.Errorf("mark summary as running failed: %v", err)
		return
	}
	defer func() {
		metadata.Summarizing = false
		metadata.UpdatedAt = time.Now()
		if err := s.memoryStore.SetMetadata(context.Background(), memoryID, metadata); err != nil {
			logger.Errorf("mark summary as finished failed: %v", err)
		}
	}()

	messages, err := s.memoryStore.GetMessages(ctx, memoryID)
	if err != nil {
		logger.Errorf("load conversation memory failed: %v", err)
		return
	}
	if len(messages) == 0 {
		logger.Info("skip summary because conversation memory is empty")
		return
	}

	result, err := s.summaryAgent.SummarizeChat(ctx, messages)
	if err != nil {
		logger.Errorf("generate chat summary failed: %v", err)
		metadata.SummaryError = err.Error()
		return
	}
	summary := strings.TrimSpace(result.Content)
	if summary == "" {
		return
	}

	err = s.AddChatMessage(ctx, appId, summary, enum.SummaryMessageType, userId)
	if err != nil {
		logger.Errorf("save chat summary failed: %v", err)
		return
	}
	if s.memoryStore != nil {
		metadata.Summary = summary
		metadata.Round++
		if result.ResponseMeta != nil && result.ResponseMeta.Usage != nil {
			usage := result.ResponseMeta.Usage
			metadata.PromptTokens += int64(usage.PromptTokens)
			metadata.CompletionTokens += int64(usage.CompletionTokens)
			metadata.TotalTokens += int64(usage.PromptTokens + usage.CompletionTokens)
			_ = s.memoryStore.AddTokenUsage(ctx, memoryID, int64(usage.PromptTokens), int64(usage.CompletionTokens))
			q := query.Use(s.db)
			_, _ = q.App.WithContext(ctx).
				Where(q.App.ID.Eq(appId), q.App.IsDelete.Eq(0)).
				UpdateSimple(
					q.App.PromptTokens.Add(int64(usage.PromptTokens)),
					q.App.CompletionTokens.Add(int64(usage.CompletionTokens)),
					q.App.TokenUsage.Add(int64(usage.PromptTokens+usage.CompletionTokens)),
				)
		}
		if err := s.memoryStore.SetMetadata(ctx, memoryID, metadata); err != nil {
			logger.Errorf("save chat summary to memory failed: %v", err)
			return
		}
		if err := s.retainRecentMessages(ctx, appId, 6); err != nil {
			logger.Errorf("trim recent conversation memory failed: %v", err)
		}
	}
}

func (s *ChatHistoryService) retainRecentMessages(ctx context.Context, appID int64, rounds int) error {
	if rounds <= 0 {
		rounds = 6
	}
	limit := rounds * 2
	q := query.Use(s.db).ChatHistory
	history, err := q.WithContext(ctx).
		Where(
			q.AppID.Eq(appID),
			q.IsDelete.Eq(0),
			q.MessageType.Neq(string(enum.SummaryMessageType)),
		).
		Order(q.CreateTime.Desc(), q.ID.Desc()).
		Limit(limit).
		Find()
	if err != nil {
		return err
	}

	memoryID := strconv.FormatInt(appID, 10)
	if err := s.memoryStore.ClearMessages(ctx, memoryID); err != nil {
		return err
	}
	for index := len(history) - 1; index >= 0; index-- {
		record := history[index]
		var message *schema.Message
		switch record.MessageType {
		case string(enum.UserMessageType):
			message = schema.UserMessage(record.Message)
		case string(enum.AIMessageType):
			message = schema.AssistantMessage(record.Message, nil)
		default:
			continue
		}
		if err := s.memoryStore.AppendMessage(ctx, message, memoryID); err != nil {
			return err
		}
	}
	return nil
}

func (s *ChatHistoryService) ListAllChatHistoryByPageForAdmin(ctx context.Context, pageNum int32, pageSize int32, queryRequest *api.NoCodeChatHistoryQueryRequest) (*response.PageResponse[*model.ChatHistory], error) {

	// Validate basic parameters.
	if pageNum <= 0 || pageSize <= 0 || pageSize > 50 {
		return nil, errorutil.ParamsError
	}
	if queryRequest == nil {
		return nil, errorutil.ParamsError
	}

	// Build base query.
	q := query.Use(s.db)
	chatHistoryQuery := q.ChatHistory.WithContext(ctx).
		Where(q.ChatHistory.ID.IsNotNull(), q.ChatHistory.IsDelete.Eq(0))

	// Apply filters.
	if queryRequest.Id > 0 {
		chatHistoryQuery = chatHistoryQuery.Where(q.ChatHistory.ID.Eq(queryRequest.Id))
	}
	if queryRequest.AppId > 0 {
		chatHistoryQuery = chatHistoryQuery.Where(q.ChatHistory.AppID.Eq(queryRequest.AppId))
	}
	if queryRequest.UserId > 0 {
		chatHistoryQuery = chatHistoryQuery.Where(q.ChatHistory.UserID.Eq(queryRequest.UserId))
	}
	if queryRequest.MessageType != "" {
		chatHistoryQuery = chatHistoryQuery.Where(q.ChatHistory.MessageType.Eq(queryRequest.MessageType))
	}
	if queryRequest.Message != "" {
		chatHistoryQuery = chatHistoryQuery.Where(
			q.ChatHistory.Message.Like("%" + queryRequest.Message + "%"),
		)
	}
	if !queryRequest.LastCreateTime.IsZero() {
		cursorCond := q.ChatHistory.CreateTime.Lt(queryRequest.LastCreateTime)
		if queryRequest.LastId > 0 {
			cursorCond = field.Or(
				q.ChatHistory.CreateTime.Lt(queryRequest.LastCreateTime),
				field.And(q.ChatHistory.CreateTime.Eq(queryRequest.LastCreateTime), q.ChatHistory.ID.Lt(queryRequest.LastId)),
			)
		}
		chatHistoryQuery = chatHistoryQuery.Where(cursorCond)
	}

	// Count total records.
	totalRow, err := chatHistoryQuery.Count()
	if err != nil {
		return nil, err
	}

	// Calculate total pages.
	totalPage := 0
	if totalRow > 0 {
		totalPage = int((totalRow + int64(pageSize) - 1) / int64(pageSize))
	}

	// Calculate offset.
	offset := int((pageNum - 1) * pageSize)

	// Execute paginated query.
	chatHistoryList, err := chatHistoryQuery.
		Order(q.ChatHistory.CreateTime.Desc(), q.ChatHistory.ID.Desc()).
		Limit(int(pageSize)).
		Offset(offset).
		Find()
	if err != nil {
		return nil, err
	}

	// Build page response.
	return &response.PageResponse[*model.ChatHistory]{
		Records:            chatHistoryList,
		PageNum:            int(pageNum),
		PageSize:           int(pageSize),
		TotalPage:          totalPage,
		TotalRow:           int(totalRow),
		OptimizeCountQuery: true,
	}, nil
}

func (s *ChatHistoryService) LoadChatHistoryToMemory(
	ctx context.Context,
	appId int64,
	memoryStore store.MemoryStore,
	maxCount int,
) (int, error) {
	q := query.Use(s.db).ChatHistory
	memoryID := strconv.FormatInt(appId, 10)

	summaryMsg, _ := q.WithContext(ctx).Where(
		q.AppID.Eq(appId),
		q.IsDelete.Eq(0),
		q.MessageType.Eq(string(enum.SummaryMessageType)),
	).Order(q.CreateTime.Desc(), q.ID.Desc()).First()

	recentQuery := q.WithContext(ctx).Where(
		q.AppID.Eq(appId),
		q.IsDelete.Eq(0),
		q.MessageType.Neq(string(enum.SummaryMessageType)),
	)
	if summaryMsg != nil {
		recentQuery = recentQuery.Where(q.CreateTime.Gt(summaryMsg.CreateTime))
	}

	recentHistory, err := recentQuery.
		Order(q.CreateTime.Desc(), q.ID.Desc()).
		Limit(maxCount).
		Find()
	if err != nil {
		return 0, err
	}

	if err := memoryStore.ClearMessages(ctx, memoryID); err != nil {
		return 0, err
	}
	if err := memoryStore.SetSummary(ctx, memoryID, ""); err != nil {
		return 0, err
	}

	loadedCount := 0
	if summaryMsg != nil {
		if err := memoryStore.SetSummary(ctx, memoryID, summaryMsg.Message); err != nil {
			return loadedCount, err
		}
		loadedCount++
	}

	for i := len(recentHistory) - 1; i >= 0; i-- {
		history := recentHistory[i]
		var msg *schema.Message
		switch history.MessageType {
		case string(enum.UserMessageType):
			msg = schema.UserMessage(history.Message)
		case string(enum.AIMessageType):
			msg = schema.AssistantMessage(history.Message, nil)
		default:
			continue
		}
		if err := memoryStore.AppendMessage(ctx, msg, memoryID); err != nil {
			return loadedCount, err
		}
		loadedCount++
	}

	return loadedCount, nil
}

func (s *ChatHistoryService) EnsureMemoryLoaded(ctx context.Context, appId int64, maxCount int) ([]*schema.Message, error) {
	if s.memoryStore == nil {
		return nil, errorutil.SystemError.WithMessage("memory store is not initialized")
	}
	memoryID := strconv.FormatInt(appId, 10)
	if err := s.restoreTotalTokenUsage(ctx, appId, memoryID); err != nil {
		return nil, err
	}
	messages, err := s.memoryStore.GetMessages(ctx, memoryID)
	if err != nil {
		return nil, err
	}
	if len(messages) > 0 {
		return messages, nil
	}
	if s.db == nil {
		return messages, nil
	}
	if _, err := s.LoadChatHistoryToMemory(ctx, appId, s.memoryStore, maxCount); err != nil {
		return nil, err
	}
	return s.memoryStore.GetMessages(ctx, memoryID)
}

func (s *ChatHistoryService) restoreTotalTokenUsage(ctx context.Context, appID int64, memoryID string) error {
	metadata, err := s.memoryStore.GetMetadata(ctx, memoryID)
	if err != nil {
		return err
	}
	if s.db == nil {
		return nil
	}
	if metadata.TotalTokens > 0 {
		return nil
	}

	q := query.Use(s.db)
	app, err := q.App.WithContext(ctx).
		Where(q.App.ID.Eq(appID), q.App.IsDelete.Eq(0)).
		First()
	if err != nil {
		return err
	}
	if app.TokenUsage <= 0 && app.PromptTokens <= 0 && app.CompletionTokens <= 0 {
		return nil
	}

	metadata.TotalTokens = app.TokenUsage
	metadata.PromptTokens = app.PromptTokens
	metadata.CompletionTokens = app.CompletionTokens
	return s.memoryStore.SetMetadata(ctx, memoryID, metadata)
}
