package router

import (
	"context"
	"time"

	"github.com/chosenlau/noCodeAI/internal/handler"
	"github.com/chosenlau/noCodeAI/internal/middleware"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/middlewares/server/recovery"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/hlog"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/hertz-contrib/cors"
)

func RegisterRoutes(h *server.Hertz, userHandler *handler.UserHandler, appHandler *handler.AppHandler, chatHistoryHandler *handler.ChatHistoryHandler, userService service.IUserService) {
	h.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://localhost:3000", "http://127.0.0.1:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
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

		userRoute.GET("/get/login", middleware.AuthMiddleware(userService), userHandler.GetLoginUserVo)
		userRoute.GET("/logout", userHandler.UserLogout)

		userRoute.POST("/add", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), userHandler.AddUser)
		userRoute.POST("/update", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), userHandler.UpdateUser)
		userRoute.POST("/delete", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), userHandler.DeleteUser)
		userRoute.GET("/list/page/vo", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), userHandler.ListUserVoByPage)
	}

	appRoute := h.Group("/app")
	{
		// 公开接口（无需登录）
		appRoute.POST("/good/list/page/vo", appHandler.ListGoodApp)
		appRoute.GET("/get/vo", middleware.AuthMiddleware(userService), appHandler.GetAppVo)

		// 用户接口（需要登录）
		appRoute.GET("/my/list/page/vo", middleware.AuthMiddleware(userService), appHandler.ListMyApp)
		appRoute.GET("/chat", middleware.AuthMiddleware(userService), appHandler.ChatToGenCode)
		appRoute.POST("/add", middleware.AuthMiddleware(userService), appHandler.AddApp)
		appRoute.POST("/update", middleware.AuthMiddleware(userService), appHandler.UpdateApp)
		appRoute.POST("/delete", middleware.AuthMiddleware(userService), appHandler.DeleteApp)

		// 管理员接口（需要管理员权限）
		appRoute.POST("/admin/update", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), appHandler.AdminUpdateApp)
		appRoute.POST("/admin/delete", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), appHandler.AdminDeleteApp)
		appRoute.GET("/admin/get/vo", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), appHandler.AdminGetAppVo)
		appRoute.POST("/admin/list/page/vo", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), appHandler.AdminListApp)
	}

	chatHistoryRoute := h.Group("/chatHistory")
	{
		// 需要管理员权限的接口
		chatHistoryRoute.POST("/admin/list/page/vo", middleware.AuthMiddleware(userService), middleware.DevRequireAdmin(), chatHistoryHandler.ListAllChatHistoryByPageForAdmin)

		chatHistoryRoute.GET("/app/:appId", middleware.AuthMiddleware(userService), chatHistoryHandler.ListAppChatHistory)
	}
	h.GET("/ping", handler.Ping)
}

func CustomRecoveryHandler(ctx context.Context, c *app.RequestContext, err interface{}, stack []byte) {
	hlog.Errorf("panic recovered:%v\n%s", err, stack)
	c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.SystemError))
	c.Abort()
}
