package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type ChatHistoryHandler struct {
	chatHistoryService service.IChatHistoryService
	userService        service.IUserService
}

func NewChatHistoryHandler(
	chatHistoryService service.IChatHistoryService,
	userService service.IUserService,
) *ChatHistoryHandler {
	return &ChatHistoryHandler{
		chatHistoryService: chatHistoryService,
		userService:        userService,
	}
}

func (h *ChatHistoryHandler) ListAppChatHistory(ctx context.Context, c *app.RequestContext) {
	// 1. 获取路径参数appId
	appIdStr := c.Param("appId")
	appId, err := strconv.ParseInt(appIdStr, 10, 64)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError.WithMessage("应用ID格式错误")))
		return
	}

	// 2. 获取查询参数pageSize，默认值为10
	pageSizeStr := c.Query("pageSize")
	pageSize := int32(10) // 默认值
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil {
			pageSize = int32(ps)
		}
	}

	// 3. 获取查询参数lastCreateTime，可选
	lastCreateTimeStr := c.Query("lastCreateTime")
	var lastCreateTime time.Time
	if lastCreateTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, lastCreateTimeStr); err == nil {
			lastCreateTime = t
		}
	}
	lastIDStr := c.Query("lastId")
	var lastID int64
	if lastIDStr != "" {
		if id, err := strconv.ParseInt(lastIDStr, 10, 64); err == nil {
			lastID = id
		}
	}

	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)

	// 5. 调用服务层方法
	result, err := h.chatHistoryService.ListAppChatHistoryByPage(ctx, appId, pageSize, lastCreateTime, lastID, userVo)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	// 6. 返回成功响应
	c.JSON(consts.StatusOK, response.NewSuccessResponse[*response.PageResponse[*model.ChatHistory]](result))
}

func (h *ChatHistoryHandler) ListAppChatHistoryByCursor(ctx context.Context, c *app.RequestContext) {
	appID, err := strconv.ParseInt(c.Param("appId"), 10, 64)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
		return
	}
	pageSize := int32(10)
	if value := c.Query("pageSize"); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
			return
		}
		pageSize = int32(parsed)
	}
	var lastCreateTime time.Time
	if value := c.Query("lastCreateTime"); value != "" {
		lastCreateTime, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
			return
		}
	}
	var lastID int64
	if value := c.Query("lastId"); value != "" {
		lastID, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
			return
		}
	}
	value, exists := c.Get(constants.UserVoKey)
	if !exists {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError))
		return
	}
	result, err := h.chatHistoryService.ListAppChatHistoryByCursor(
		ctx, appID, pageSize, lastCreateTime, lastID, value.(*api.UserVo),
	)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[*api.CursorResponse](result))
}

func (h *ChatHistoryHandler) ListAllChatHistoryByPageForAdmin(ctx context.Context, c *app.RequestContext) {
	// 1. 绑定请求参数
	req := &api.NoCodeChatHistoryQueryRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
		return
	}

	// 2. 获取分页参数
	pageNum := int32(1)   // 默认值
	pageSize := int32(10) // 默认值

	// 3. 调用服务层方法
	result, err := h.chatHistoryService.ListAllChatHistoryByPageForAdmin(ctx, pageNum, pageSize, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	// 4. 返回成功响应
	c.JSON(consts.StatusOK, response.NewSuccessResponse(result))
}
