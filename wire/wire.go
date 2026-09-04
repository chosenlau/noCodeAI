package wire

import (
	"fmt"

	"github.com/chosenlau/noCodeAI/config"

	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/router"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/google/wire"
)

func initServer() *server.Hertz {
	cfg := config.GlobalConfig

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)),
		server.WithBasePath(cfg.Server.ContextPath),
	)
	router.RegisterRoutes(h)
	return h
}

var dbSet = wire.NewSet(
	dal.InitDB, // 提供 *gorm.DB
)

func InitializeApp() (*server.Hertz, error) {
	panic(wire.Build(
		initServer,
	))
}
