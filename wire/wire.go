//go:build wireinject
// +build wireinject

package wire

import (
	"fmt"

	"github.com/chosenlau/noCodeAI/config"

	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/handler"
	"github.com/chosenlau/noCodeAI/internal/logic"
	"github.com/chosenlau/noCodeAI/internal/router"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/google/wire"
)

func ProvideChatModel(cfg *config.Config) (agent.ChatModelWrapperAdaptor, error) {
	switch cfg.AI.Provider {
	case "claude":
		return llm.NewClaudeChatModel(cfg), nil // 返回 Claude 实现
	case "openai":
		return llm.NewOpenAIChatModel(cfg), nil // 返回 OpenAI 实现
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.AI.Provider)
	}
}

var configSet = wire.NewSet(
	config.InitConfig,
)
var dbSet = wire.NewSet(
	dal.InitDB,
	dal.InitRedis,
)
var serviceSet = wire.NewSet(
	logic.NewAppService,
	wire.Bind(new(service.IAppService), new(*logic.AppService)),
	logic.NewUserService,
	wire.Bind(new(service.IUserService), new(*logic.UserService)),
	logic.NewChatHistoryService,
	wire.Bind(new(service.IChatHistoryService), new(*logic.ChatHistoryService)),
)

var handlerSet = wire.NewSet(
	handler.NewUserHandler,
	handler.NewAppHandler,
	handler.NewChatHistoryHandler,
)

var llmSet = wire.NewSet(
	ProvideChatModel,
)

func initServer(cfg *config.Config, userHandler *handler.UserHandler, appHandler *handler.AppHandler, chatHistoryHandler *handler.ChatHistoryHandler, userService service.IUserService) *server.Hertz {

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)),
		server.WithBasePath(cfg.Server.ContextPath),
	)
	router.RegisterRoutes(h, userHandler, appHandler, chatHistoryHandler, userService)
	return h
}

func InitializeApp() (*server.Hertz, error) {
	panic(wire.Build(
		initServer,
		configSet,
		dbSet,
		serviceSet,
		handlerSet,
		llmSet,

		core.NewNoCodeAIGenFacade,
		saver.NewCodeSaver,
		agent.NewCodeGenAgentFactory,
	))
}
