package middleware

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func AuthMiddleware(userService service.IUserService) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 1. 从 Cookie 中提取 Session ID
		cookieBytes := c.Request.Header.Cookie(constants.UserLoginState)
		if len(cookieBytes) == 0 {
			// 未携带 Cookie，直接拦截并返回未登录错误
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](
				errorutil.NotLoginError.WithMessage("尚未登录或登录已过期"),
			))
			c.Abort() // 核心：终止后续 Handler 的执行
			return
		}

		sessionId := string(cookieBytes)

		// 2. 校验 Session ID 的有效性
		// 注意：这里复用了你的 GetLoginUserVo，如果该方法内部做了 Redis 校验，非常完美
		userVo, err := userService.GetLoginUserVo(ctx, sessionId)
		if err != nil {
			// 校验失败（Session已过期或伪造），拦截请求
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](
				errorutil.NotLoginError.WithMessage("登录状态无效，请重新登录"),
			))
			c.Abort()
			return
		}

		// 3. 上下文注入 (Context Injection)
		// 将获取到的用户信息直接挂载到当前请求的 Context 中
		// 这样下游的 Handler 就完全不需要再去查数据库或 Redis 了
		c.Set(constants.UserVoKey, userVo)
		// 建议再单独存一个 UserID，方便下游直接拿来查表
		// c.Set(constants.ContextUserId, userVo.ID)

		// 4. 鉴权通过，放行请求，执行下一个 Handler
		c.Next(ctx)
	}
}

func DevRequireAdmin() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		v, exist := c.Get(constants.UserVoKey)
		if !exist {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](
				errorutil.NotLoginError.WithMessage("尚未登录或登录已过期"),
			))
			c.Abort()
			return
		}
		userVO := v.(*api.UserVo)
		if userVO.UserRole != enum.RoleAdmin.GetRoleText(){
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](
				errorutil.NotLoginError.WithMessage("无权限"),
			))
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}
