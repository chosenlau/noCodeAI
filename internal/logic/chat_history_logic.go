package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/logger"
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
	db *gorm.DB
}

func NewChatHistoryService(db *gorm.DB) *ChatHistoryService {
	return &ChatHistoryService{
		db: db,
	}
}

func (s *ChatHistoryService) ListAppChatHistoryByPage(ctx context.Context,
	appId int64, pageSize int32, lastCreateTime time.Time, lastID int64, loginUser *api.UserVo) (*response.PageResponse[*model.ChatHistory], error) {

	// 1. 校验基本参数
	if appId == 0 || appId < 0 || pageSize <= 0 || pageSize > 50 {
		return nil, errorutil.ParamsError
	}
	if loginUser == nil {
		return nil, errorutil.NotLoginError
	}

	// 2. 校验用户角色是否为管理员或者应用创建者
	q := query.Use(s.db)
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(appId), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return nil, err
	}
	if app.UserID != loginUser.ID && loginUser.UserRole != string(enum.RoleAdmin) {
		return nil, errorutil.NotAuthError
	}

	// 3. 构建查询条件
	chatHistoryQuery := q.ChatHistory.WithContext(ctx).
		Where(q.ChatHistory.AppID.Eq(appId), q.ChatHistory.IsDelete.Eq(0)).
		Where(q.ChatHistory.MessageType.Neq(string(enum.SummaryMessageType)))

	// 4. 处理时间过滤（游标分页）
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

	// 5. 查询总记录数
	totalRow, err := chatHistoryQuery.Count()
	if err != nil {
		return nil, err
	}

	// 6. 计算总页数
	totalPage := 0
	if totalRow > 0 {
		totalPage = int((totalRow + int64(pageSize) - 1) / int64(pageSize))
	}
	//TODO:cursor分页
	// 7. 分页查询应用的聊天记录
	chatHistoryList, err := chatHistoryQuery.
		Order(q.ChatHistory.CreateTime.Desc(), q.ChatHistory.ID.Desc()).
		Limit(int(pageSize)).
		Find()
	if err != nil {
		return nil, err
	}

	// 8. 构建并返回分页响应
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
	// 1. 校验应用ID
	if appId == 0 || appId < 0 {
		return errorutil.ParamsError.WithMessage("应用ID不能为空")
	}

	// 2. 删除该应用的所有对话记录
	q := query.Use(s.db)
	_, err := q.ChatHistory.WithContext(ctx).
		Where(q.ChatHistory.AppID.Eq(appId), q.ChatHistory.IsDelete.Eq(0)).
		Update(q.ChatHistory.IsDelete, 1)
	if err != nil {
		return err
	}

	return nil
}

func (s *ChatHistoryService) AddChatMessage(ctx context.Context, appId int64,
	message string, messageType enum.ChatHistoryMessageTypeEnum, userId int64) error {

	// 1. 校验参数
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

		// 2. 如果当前是用户消息，开启新的一轮
		if messageType == enum.UserMessageType {
			turnNumber += 1
		}

		// 3. 生成雪花算法ID
		chatMessageId, err := snowflake.GenerateSnowFlakeId()
		if err != nil {
			return err
		}

		// 4. 创建对话记录
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

	// 5. 当对话轮次达到20轮且为AI消息时，异步生成总结
	if turnNumber >= 20 && messageType == enum.AIMessageType {
		go s.generateSummary(context.Background(), appId, userId)
	}

	return nil
}

// generateSummary 生成对话总结
func (s *ChatHistoryService) generateSummary(ctx context.Context, appId int64, userId int64) {
	// 1. 获取历史对话记录（按时间正序）
	historyList, err := query.Use(s.db).WithContext(ctx).ChatHistory.
		Where(query.ChatHistory.AppID.Eq(appId), query.ChatHistory.IsDelete.Eq(0)).
		Order(query.ChatHistory.CreateTime.Asc(), query.ChatHistory.ID.Asc()).
		Find()
	if err != nil {
		logger.Errorf("获取历史对话失败: %v\n", err)
		return
	}

	// 2. 构建对话历史字符串
	var chatHistoryBuilder strings.Builder
	for _, history := range historyList {
		if history.MessageType == string(enum.UserMessageType) {
			chatHistoryBuilder.WriteString(fmt.Sprintf("用户: %s\n", history.Message))
		} else if history.MessageType == string(enum.AIMessageType) {
			chatHistoryBuilder.WriteString(fmt.Sprintf("AI: %s\n", history.Message))
		}
	}

	//TODO:An agent to generate summary
	err = s.AddChatMessage(ctx, appId, chatHistoryBuilder.String(), enum.SummaryMessageType, userId)
	if err != nil {
		logger.Errorf("对话总结保存失败: %v\n", err)
	}
}

func (s *ChatHistoryService) ListAllChatHistoryByPageForAdmin(ctx context.Context, pageNum int32, pageSize int32, queryRequest *api.NoCodeChatHistoryQueryRequest) (*response.PageResponse[*model.ChatHistory], error) {

	// 1. 校验基本参数
	if pageNum <= 0 || pageSize <= 0 || pageSize > 50 {
		return nil, errorutil.ParamsError
	}
	if queryRequest == nil {
		return nil, errorutil.ParamsError
	}

	// 2. 构建基础查询
	chatHistoryQuery := query.Use(s.db).ChatHistory.WithContext(ctx).
		Where(query.ChatHistory.ID.IsNotNull(), query.ChatHistory.IsDelete.Eq(0))

	// 3. 动态添加查询条件
	if queryRequest.Id > 0 {
		chatHistoryQuery = chatHistoryQuery.Where(query.ChatHistory.ID.Eq(queryRequest.Id))
	}
	if queryRequest.AppId > 0 {
		chatHistoryQuery = chatHistoryQuery.Where(query.ChatHistory.AppID.Eq(queryRequest.AppId))
	}
	if queryRequest.UserId > 0 {
		chatHistoryQuery = chatHistoryQuery.Where(query.ChatHistory.UserID.Eq(queryRequest.UserId))
	}
	if queryRequest.MessageType != "" {
		chatHistoryQuery = chatHistoryQuery.Where(query.ChatHistory.MessageType.Eq(queryRequest.MessageType))
	}
	if queryRequest.Message != "" {
		chatHistoryQuery = chatHistoryQuery.Where(
			query.ChatHistory.Message.Like("%" + queryRequest.Message + "%"),
		)
	}
	if !queryRequest.LastCreateTime.IsZero() {
		cursorCond := query.ChatHistory.CreateTime.Lt(queryRequest.LastCreateTime)
		if queryRequest.LastId > 0 {
			cursorCond = field.Or(
				query.ChatHistory.CreateTime.Lt(queryRequest.LastCreateTime),
				field.And(query.ChatHistory.CreateTime.Eq(queryRequest.LastCreateTime), query.ChatHistory.ID.Lt(queryRequest.LastId)),
			)
		}
		chatHistoryQuery = chatHistoryQuery.Where(cursorCond)
	}

	// 4. 查询总记录数
	totalRow, err := chatHistoryQuery.Count()
	if err != nil {
		return nil, err
	}

	// 5. 计算总页数
	totalPage := 0
	if totalRow > 0 {
		totalPage = int((totalRow + int64(pageSize) - 1) / int64(pageSize))
	}

	// 6. 计算偏移量
	offset := int((pageNum - 1) * pageSize)

	// 7. 执行分页查询
	chatHistoryList, err := chatHistoryQuery.
		Order(query.ChatHistory.CreateTime.Desc(), query.ChatHistory.ID.Desc()).
		Limit(int(pageSize)).
		Offset(offset).
		Find()
	if err != nil {
		return nil, err
	}

	// 8. 构建并返回分页响应
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

	// 1. 获取最新的一条摘要 (Limit 1)
	var summaryMsg *model.ChatHistory
	summaryMsg, _ = q.WithContext(ctx).Where(
		q.AppID.Eq(appId),
		q.IsDelete.Eq(0),
		q.MessageType.Eq(string(enum.SummaryMessageType)),
	).Order(q.CreateTime.Desc(), q.ID.Desc()).First() // 直接拿最新的一条

	// 2. 动态构建“近期对话”查询条件
	recentQuery := q.WithContext(ctx).Where(
		q.AppID.Eq(appId),
		q.IsDelete.Eq(0),
		q.MessageType.Neq(string(enum.SummaryMessageType)), // 排除摘要本身，只查纯对话
	)

	// 🌟 核心逻辑修复：如果有摘要，只查在摘要生成【之后】产生的新对话
	if summaryMsg != nil {
		recentQuery = recentQuery.Where(q.CreateTime.Gt(summaryMsg.CreateTime))
	}

	// 3. 执行查询，使用 Offset(1) 剔除当次请求引发的未完成脏数据
	recentHistory, err := recentQuery.
		Order(q.CreateTime.Desc(), q.ID.Desc()).
		Offset(1).
		Limit(maxCount).
		Find()

	if err != nil {
		return 0, err
	}

	// 4. 清理旧缓存
	if err := memoryStore.ClearMessages(ctx); err != nil {
		return 0, err
	}

	loadedCount := 0

	// 5. 组装环节一：如果有摘要，必须【最先】塞入 Redis 作为底座（System Message）
	if summaryMsg != nil {
		err = memoryStore.AppendMessage(ctx, schema.SystemMessage(summaryMsg.Message))
		if err != nil {
			return loadedCount, err
		}
		loadedCount++
	}

	// 6. 组装环节二：将查询到的近期对话，翻转时序（从旧到新）追加到 Redis
	for i := len(recentHistory) - 1; i >= 0; i-- {
		history := recentHistory[i]

		var msg *schema.Message
		switch history.MessageType {
		case string(enum.UserMessageType):
			msg = schema.UserMessage(history.Message)
		case string(enum.AIMessageType):
			msg = schema.AssistantMessage(history.Message, nil)
		default:
			continue // 脏数据跳过
		}

		if err := memoryStore.AppendMessage(ctx, msg); err != nil {
			return loadedCount, err
		}
		loadedCount++
	}

	return loadedCount, nil
}
