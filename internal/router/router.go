package router

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/chosenlau/noCodeAI/internal/handler"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/cors"
)

func RegisterRoutes(h *server.Hertz, userHandler *handler.UserHandler) {
	h.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"*"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	h.Use(recovery.Recovery(recovery.WithRecoveryHandler(CustomRecoveryHandler)))

	userRoute := h.Group("/user")
	{
		userRoute.POST("/register", userHandler.UserRegister)
		userRoute.POST("/login", userHandler.UserLogin)
		userRoute.GET("/get/vo", userHandler.GetUserByID)

		userRoute.GET("/get/login",
			AuthMiddleware(), userHandler.GetLoginUserVo)
		userRoute.GET("/logout", AuthMiddleware(), userHandler.UserLogout)

		userRoute.POST("/add", AuthMiddleware(), DevRequireAdmin(), userHandler.AddUser)
		userRoute.POST("/update", AuthMiddleware(), DevRequireAdmin(), userHandler.UpdateUser)
		userRoute.POST("/delete", AuthMiddleware(), DevRequireAdmin(), userHandler.DeleteUser)
		userRoute.GET("/list/page/vo", AuthMiddleware(), DevRequireAdmin(), userHandler.ListUserVoByPage)
	}

	h.GET("/ping", handler.Ping)
}

func CustomRecoveryHandler(ctx context.Context, c *app.RequestContext, err interface{}, stack []byte) {
	hlog.Errorf("panic recovered:%v\n%s", err, stack)
	c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError.WithMessage(fmt.Sprintf("%v", err))))
	c.Abort()
}

func AuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		cookieUserID := c.Cookie("user_id")
		cookieUserRole := c.Cookie("user_role")

		if len(cookieUserID) == 0 || len(cookieUserRole) == 0 {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError.WithMessage("Login status not detected.")))
			c.Abort()
			return
		}

		userID, err := strconv.ParseInt(string(cookieUserID), 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError.WithMessage("Invalid login credentials.")))
			c.Abort()
			return
		}

		ctx = context.WithValue(ctx, constants.UserIDKey, userID)
		ctx = context.WithValue(ctx, constants.UserRoleKey, string(cookieUserRole))
		c.Next(ctx)
	}
}

func DevRequireAdmin() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		role, ok := ctx.Value(constants.UserRoleKey).(string)
		if !ok || role != "admin" {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotAuthError.WithMessage("Admin privileges required.")))
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}
