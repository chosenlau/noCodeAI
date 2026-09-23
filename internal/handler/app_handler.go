package handler

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/request"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/eino/schema"
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

func (a *AppHandler) GetSourceCode(ctx context.Context, c *app.RequestContext) {
	appID, err := strconv.ParseInt(c.Param("appId"), 10, 64)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError))
		return
	}
	generationType := string(c.Query("generationType"))
	value, exists := c.Get(constants.UserVoKey)
	if !exists {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError))
		return
	}
	files, err := a.appService.GetSourceCode(ctx, appID, generationType, value.(*api.UserVo))
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[map[string]string](files))
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

func (a *AppHandler) GraphToGenCode(ctx context.Context, c *app.RequestContext) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	var req api.NoCodeGenCodeRequest
	if err := c.BindAndValidate(&req); err != nil {
		sendSseErrorAndExit(c, "参数解析失败: "+err.Error())
		return
	}

	value, exists := c.Get(constants.UserVoKey)
	if !exists {
		sendSseErrorAndExit(c, "用户未登录")
		return
	}
	userVo := value.(*api.UserVo)

	stream, workflowContext, err := a.appService.GraphToGenCode(ctx, req.AppId, req.Message, userVo)
	if err != nil {
		sendSseErrorAndExit(c, err.Error())
		return
	}

	w := sse.NewWriter(c)
	lastEventID := sse.GetLastEventID(&c.Request)

	// 直接使用匿名结构体定义 Channel，无需在外部声明 type
	sseChan := make(chan struct {
		chunk *schema.Message
		err   error
	}, 5)
	streamDone := make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			stream.Close()
		case <-streamDone:
		}
	}()

	go func() {
		defer close(streamDone)
		defer close(sseChan)
		defer stream.Close()

		for {
			chunk, err := stream.Recv()

			// 组装匿名结构体数据
			res := struct {
				chunk *schema.Message
				err   error
			}{chunk, err}

			select {
			case sseChan <- res:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-heartbeat.C:
			_ = w.WriteEvent(lastEventID, "heartbeat", []byte("ping"))
			c.Flush()

		case res, ok := <-sseChan:
			if !ok {
				return
			}
			// 直接判断 res.err 是否为 io.EOF
			if res.err == io.EOF {
				data, marshalErr := json.Marshal(workflowContext.CodeContent)
				if marshalErr != nil {
					_ = w.WriteEvent(lastEventID, "error", []byte(marshalErr.Error()))
				} else {
					_ = w.WriteEvent(lastEventID, "code_completed", data)
				}
				if workflowContext.Description != "" {
					_ = w.WriteEvent(
						lastEventID,
						"description_completed",
						[]byte(workflowContext.Description),
					)
				}
				_ = w.WriteEvent(lastEventID, "done", []byte{1})
				c.Flush()
				return
			}

			if res.err != nil {
				_ = w.WriteEvent(lastEventID, "error", []byte(res.err.Error()))
				_ = w.WriteEvent(lastEventID, "done", []byte{1})
				c.Flush()
				return
			}

			if res.chunk != nil && res.chunk.Content != "" {
				eventName := "ai_response"
				var envelope struct {
					Type        string `json:"type"`
					StepNumber  int    `json:"stepNumber"`
					CurrentStep string `json:"currentStep"`
				}
				if json.Unmarshal([]byte(res.chunk.Content), &envelope) == nil {
					switch envelope.Type {
					case "step_started":
						eventName = "step_started"
					case "step_completed":
						eventName = "step_completed"
					case "tool_request":
						eventName = "tool_request"
					case "tool_executed":
						eventName = "tool_executed"
					case "ai_response":
						eventName = "ai_response"
					}
					if eventName == "ai_response" && (envelope.CurrentStep != "" || envelope.StepNumber > 0) {
						eventName = "step_completed"
					}
				}
				_ = w.WriteEvent(lastEventID, eventName, []byte(res.chunk.Content))
				c.Flush()
			}
		}
	}
}

// 辅助函数：简化初始错误返回的重复代码
func sendSseErrorAndExit(c *app.RequestContext, errMsg string) {
	w := sse.NewWriter(c)
	lastEventID := sse.GetLastEventID(&c.Request)
	_ = w.WriteEvent(lastEventID, "error", []byte(errMsg))
	_ = w.WriteEvent(lastEventID, "done", []byte{1})
	c.Flush()
}
