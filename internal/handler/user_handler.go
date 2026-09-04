package handler

import (
	"context"
	"strconv"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type UserHandler struct {
	userService service.IUserService
}

func NewUserHandler(userService service.IUserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) UserRegister(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeRegisterRequest{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	userID, err := h.userService.UserRegister(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[int64](userID))

}

func (h *UserHandler) UserLogin(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeLoginRequest{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	userVo, err := h.userService.UserLogin(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	// TODO(auth): 当前为开发阶段简易实现，生产环境需迁移至 JWT 或服务端 Session 机制，防止伪造 user_id
	c.SetCookie("user_id", strconv.FormatInt(userVo.ID, 10),
		86400, "/", "", protocol.CookieSameSiteLaxMode, false, true)

	c.JSON(consts.StatusOK, response.NewSuccessResponse[*api.UserVo](userVo))
}
func (h *UserHandler) GetLoginUserVo(ctx context.Context, c *app.RequestContext) {
	cookieVal := c.Cookie("user_id")
	if len(cookieVal) == 0 {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError.WithMessage("Login status not detected.")))
		return
	}

	userID, err := strconv.ParseInt(string(cookieVal), 10, 64)
	if err != nil || userID <= 0 {
		// Clear the invalid or corrupted cookie
		h.clearUserCookie(c)
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError.WithMessage("Invalid login credentials, please log in again.")))
		return
	}

	userVo, err := h.userService.GetLoginUserVo(ctx, userID)
	if err != nil {
		// Clear stale credentials if the user no longer exists or query fails
		h.clearUserCookie(c)
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}

	c.JSON(consts.StatusOK, response.NewSuccessResponse[*api.UserVo](userVo))
}

func (h *UserHandler) UserLogout(ctx context.Context, c *app.RequestContext) {
	h.clearUserCookie(c)
	c.JSON(consts.StatusOK, response.NewSuccessResponse[any](true))
}
func (h *UserHandler) clearUserCookie(c *app.RequestContext) {
	// Setting MaxAge to -1 instructs the browser to immediately delete the cookie
	c.SetCookie("user_id", "", -1, "/", "", protocol.CookieSameSiteLaxMode, false, true)
}
func (h *UserHandler) AddUser(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeUserAddRequest{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	userID, err := h.userService.AddUser(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse[int64](userID))
}
func (h *UserHandler) GetUserByID(ctx context.Context, c *app.RequestContext) {
	req := &api.UserVo{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	if req.ID <= 0 {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError.WithMessage("invalid id")))
		return
	}
	user, err := h.userService.GetUserByID(ctx, req.ID)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse(*user))
}

func (h *UserHandler) UpdateUser(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeUserUpdateRequest{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	if req.Id <= 0 {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError.WithMessage("invalid id")))
		return
	}
	err := h.userService.UpdateUser(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse(true))
}

func (h *UserHandler) DeleteUser(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeUserUpdateRequest{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	if req.Id <= 0 {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.ParamsError.WithMessage("invalid id")))
		return
	}
	err := h.userService.DeleteUser(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse(true))
}

func (h *UserHandler) ListUserVoByPage(ctx context.Context, c *app.RequestContext) {
	req := &api.NoCodeUserQueryRequest{}
	if err := c.BindAndValidate(req); err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	resp, err := h.userService.ListUsersVoByPage(ctx, req)
	if err != nil {
		c.JSON(consts.StatusOK, response.NewErrorResponse[any](err))
		return
	}
	c.JSON(consts.StatusOK, response.NewSuccessResponse(resp))
}
