package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/request"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
)

type AppHandler struct {
	appService         service.IAppService
	userService        service.IUserService
	chatHistoryService service.IChatHistoryService
}

func NewAppHandler(
	appService service.IAppService,
	userService service.IUserService,
	chatHistoryService service.IChatHistoryService,
) *AppHandler {
	return &AppHandler{
		appService:         appService,
		userService:        userService,
		chatHistoryService: chatHistoryService,
	}
}

func (a *AppHandler) AddApp(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeAppAddRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)

	appId, err := a.appService.AddApp(ctx, req, userVo.ID)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	// 4. 返回成功响应
	c.JSON(consts.StatusOK, response.NewSuccessResponse[string](strconv.Itoa(int(appId))))
}

func (a *AppHandler) UpdateApp(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeAppUpdateRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)
	success, err := a.appService.UpdateApp(ctx, req, userVo.ID)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[bool](success))
}

func (a *AppHandler) DeleteApp(ctx context.Context, c *app.RequestContext) {
	req := &request.DeleteRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	success, err := a.appService.DeleteApp(ctx, int64(req.Id), userVo.ID)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[bool](success))
}

func (a *AppHandler) GetAppVo(ctx context.Context, c *app.RequestContext) {
	// 1. 获取查询参数
	id := c.Query("id")
	if id == "" {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
		return
	}
	idInt64, _ := strconv.ParseInt(id, 10, 64)

	// 2. 获取当前登录用户
	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)

	// 3. 调用服务层获取应用详情
	appVo, err := a.appService.GetAppVo(ctx, idInt64, userVo.ID)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	// 4. 返回应用详情
	c.JSON(consts.StatusOK, response.NewSuccessResponse[api.AppVo](appVo))
}

func (a *AppHandler) ListMyApp(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeAppMyListRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)
	pageResponse, err := a.appService.ListMyApp(ctx, req, userVo.ID)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse(pageResponse))
}

func (a *AppHandler) ListGoodApp(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeAppFeaturedListRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	pageResponse, err := a.appService.ListGoodApp(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse(pageResponse))
}

func (a *AppHandler) AdminUpdateApp(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeAppAdminUpdateRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	success, err := a.appService.AdminUpdateApp(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[bool](success))
}

func (a *AppHandler) AdminDeleteApp(ctx context.Context, c *app.RequestContext) {
	req := &request.DeleteRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	success, err := a.appService.AdminDeleteApp(ctx, int64(req.Id))
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[bool](success))
}

func (a *AppHandler) AdminGetAppVo(ctx context.Context, c *app.RequestContext) {
	id := c.Query("id")
	if id == "" {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
		return
	}
	idInt64, _ := strconv.ParseInt(id, 10, 64)
	appVo, err := a.appService.AdminGetAppVo(ctx, idInt64)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[api.AppVo](appVo))
}

func (a *AppHandler) AdminListApp(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeAppAdminListRequest{}
	err := c.BindAndValidate(req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	pageResponse, err := a.appService.AdminListApp(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[*response.PageResponse[*model.App]](pageResponse))
}

func (a *AppHandler) ChatToGenCode(ctx context.Context, c *app.RequestContext) {
	// 1. 设置 SSE 响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 2. 获取请求参数
	appIdStr := c.Query("appId")
	w := sse.NewWriter(c)
	lastEventID := sse.GetLastEventID(&c.Request)

	if appIdStr == "" {
		_ = w.WriteEvent(lastEventID, "error", []byte("appId不能为空"))
		_ = w.WriteEvent(lastEventID, "done", []byte{1})
		return
	}
	message := c.Query("message")
	if message == "" {
		_ = w.WriteEvent(lastEventID, "error", []byte("消息不能为空"))
		_ = w.WriteEvent(lastEventID, "done", []byte{1})
		return
	}

	// 3. 获取当前登录用户
	v, exists := c.Get(constants.UserVoKey)
	if !exists {
		// 理论上只要过了中间件，这里一定存在。属于系统防御性兜底
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
		return
	}
	// 2. 断言类型并返回
	userVo := v.(*api.UserVo)
	// 4. 转换应用ID
	appId, err := strconv.ParseInt(appIdStr, 10, 64)
	if err != nil {
		_ = w.WriteEvent(lastEventID, "error", []byte(fmt.Sprintf("%v", err)))
		_ = w.WriteEvent(lastEventID, "done", []byte{1})
		return
	}

	// 5. 获取流数据
	streamResp, err := a.appService.ChatToGenCode(ctx, appId, message, userVo)
	if err != nil {
		_ = w.WriteEvent(lastEventID, "error", []byte(fmt.Sprintf("%v", err)))
		_ = w.WriteEvent(lastEventID, "done", []byte{1})
		return
	}
	defer streamResp.Close()

	// 6. 流式返回数据
	var aiResponseBuilder strings.Builder
	for {
		select {
		case <-ctx.Done():
			logger.Info("连接中断")
			_ = w.WriteEvent(lastEventID, "done", []byte{1})
			return
		default:
		}

		chunk, err := streamResp.Recv()
		if err == io.EOF || errors.Is(err, context.Canceled) {
			break
		}
		if err != nil {
			_ = w.WriteEvent(lastEventID, "error", []byte(fmt.Sprintf("%v", err)))
			_ = w.WriteEvent(lastEventID, "done", []byte{1})
			return
		}
		aiResponseBuilder.WriteString(chunk.Content)

		// 7. 发送SSE事件
		wrapper := &map[string]string{
			"d": chunk.Content,
		}
		data, err := json.Marshal(wrapper)
		if err != nil {
			logger.Errorf("序列化数据失败: %v\n", err)
			continue
		}

		err = w.WriteEvent(lastEventID, "message", data)
		if err != nil {
			_ = w.WriteEvent(lastEventID, "error", []byte(fmt.Sprintf("%v", err)))
			_ = w.WriteEvent(lastEventID, "done", []byte{1})
			return
		}
	}
	err = a.chatHistoryService.AddChatMessage(ctx, appId, aiResponseBuilder.String(), enum.AIMessageType, userVo.ID)
	if err != nil {
		logger.Errorf("保存对话历史失败: %v\n", err)
	}
	// 8. 发送完成事件
	_ = w.WriteEvent(lastEventID, "done", []byte{1})
}
