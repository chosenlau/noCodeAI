package router

import (
	"context"
	"fmt"
	"time"

	"github.com/chosenlau/noCodeAI/internal/handler"
	"github.com/chosenlau/noCodeAI/internal/middleware"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/cors"
)

func RegisterRoutes(h *server.Hertz, userHandler *handler.UserHandler, appHandler *handler.AppHandler) {
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

		userRoute.GET("/get/login", middleware.AuthMiddleware(), userHandler.GetLoginUserVo)
		userRoute.GET("/logout", middleware.AuthMiddleware(), userHandler.UserLogout)

		userRoute.POST("/add", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), userHandler.AddUser)
		userRoute.POST("/update", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), userHandler.UpdateUser)
		userRoute.POST("/delete", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), userHandler.DeleteUser)
		userRoute.GET("/list/page/vo", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), userHandler.ListUserVoByPage)
	}

	appRoute := h.Group("/app")
	{
		// 公开接口（无需登录）
		appRoute.POST("/good/list/page/vo", appHandler.ListGoodApp)
		appRoute.GET("/get/vo", middleware.AuthMiddleware(), appHandler.GetAppVo)

		// 用户接口（需要登录）
		appRoute.POST("/my/list/page/vo", middleware.AuthMiddleware(), appHandler.ListMyApp)
		appRoute.POST("/add", middleware.AuthMiddleware(), appHandler.AddApp)
		appRoute.POST("/update", middleware.AuthMiddleware(), appHandler.UpdateApp)
		appRoute.POST("/delete", middleware.AuthMiddleware(), appHandler.DeleteApp)

		// 管理员接口（需要管理员权限）
		appRoute.POST("/admin/update", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), appHandler.AdminUpdateApp)
		appRoute.POST("/admin/delete", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), appHandler.AdminDeleteApp)
		appRoute.GET("/admin/get/vo", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), appHandler.AdminGetAppVo)
		appRoute.POST("/admin/list/page/vo", middleware.AuthMiddleware(), middleware.DevRequireAdmin(), appHandler.AdminListApp)
	}

	h.GET("/ping", handler.Ping)
}

func CustomRecoveryHandler(ctx context.Context, c *app.RequestContext, err interface{}, stack []byte) {
	hlog.Errorf("panic recovered:%v\n%s", err, stack)
	c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError.WithMessage(fmt.Sprintf("%v", err))))
	c.Abort()
}
