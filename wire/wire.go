package wire

import (
	"fmt"

	"github.com/chosenlau/noCodeAI/config"

	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/handler"
	"github.com/chosenlau/noCodeAI/internal/router"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/google/wire"
)

var configSet = wire.NewSet(
	config.InitConfig,
)
var dbSet = wire.NewSet(
	dal.InitDB,
)
var serviceSet = wire.NewSet(
	service.NewUserService,
)

var handlerSet = wire.NewSet(
	handler.NewUserHandler,
)

func initServer(cfg *config.Config, userHandler *handler.UserHandler) *server.Hertz {

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)),
		server.WithBasePath(cfg.Server.ContextPath),
	)
	router.RegisterRoutes(h, userHandler)
	return h
}

func InitializeApp() (*server.Hertz, error) {
	panic(wire.Build(
		initServer,
		configSet,
		dbSet,
		serviceSet,
		handlerSet,
	))
}
