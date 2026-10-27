//go:build wireinject
// +build wireinject

package wire

import (
	"context"
	"fmt"

	"github.com/chosenlau/noCodeAI/config"

	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/workflow"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/dal/taskstore"
	"github.com/chosenlau/noCodeAI/internal/handler"
	"github.com/chosenlau/noCodeAI/internal/logic"
	"github.com/chosenlau/noCodeAI/internal/router"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"time"
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

func ProvideSimpleWorkflow(
	chatModel agent.ChatModelWrapperAdaptor,
	aiCodeGenFacade *core.NoCodeAIGenFacade,
) *workflow.SimpleWorkflow {
	routingAgent := agent.NewCodeGenTypeRoutingAgent(chatModel, nil, nil)
	qualityAgent := agent.NewCodeQualityCheckAgent(chatModel, nil, nil)
	return workflow.NewSimpleWorkflow(
		node.NewRouterNode(routingAgent),
		node.NewPromptEnhancerNode(),
		node.NewCodeGeneratorNode(aiCodeGenFacade),
		node.NewCodeQualityCheckNode(qualityAgent),
	)
}

func ProvideMemoryStore(client *redis.Client) store.MemoryStore {
	return store.NewRedisMemoryStore(client, 20, 24*time.Hour)
}

func ProvideToolManager() (*aitools.ToolManager, error) {
	return aitools.NewToolManager()
}

// Preserve the existing agent configuration (no checkpoint or middleware yet).
func ProvideHTMLAgent(model agent.ChatModelWrapperAdaptor) *agent.HtmlCodeGenAgent {
	return agent.NewHtmlCodeGenAgent(model, nil, nil, nil)
}
func ProvideMultiFileAgent(model agent.ChatModelWrapperAdaptor) *agent.MultiFileCodeGenAgent {
	return agent.NewMultiFileCodeGenAgent(model, nil, nil, nil)
}
func ProvideVueAgent(model agent.ChatModelWrapperAdaptor, tools *aitools.ToolManager) *agent.VueCodeGenAgent {
	return agent.NewVueCodeGenAgent(model, nil, nil, tools, nil)
}
func ProvideSummaryAgent(model agent.ChatModelWrapperAdaptor) *agent.ChatSummaryAgent {
	return agent.NewChatSummaryAgent(model, nil, nil)
}
func ProvideChatAgent(model agent.ChatModelWrapperAdaptor) *agent.ChatAgent {
	return agent.NewChatAgent(model, nil, nil)
}

func ProvideTaskOptions(cfg *config.Config) task.Options {
	return task.Options{
		Enabled: cfg.Features.DetachedGraphTask, WorkerCount: cfg.Task.WorkerCount,
		PollInterval: cfg.Task.PollInterval, ShutdownTimeout: cfg.Task.ShutdownTimeout,
		OutboxBatchSize: cfg.Task.OutboxBatchSize,
	}
}

func ProvideTaskManager(db *gorm.DB, options task.Options) *task.Manager {
	return task.NewManager(taskstore.NewGormStore(db), options)
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

func initServer(cfg *config.Config, userHandler *handler.UserHandler, appHandler *handler.AppHandler, chatHistoryHandler *handler.ChatHistoryHandler, userService service.IUserService, redisClient *redis.Client, taskManager *task.Manager) *server.Hertz {

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)),
		server.WithBasePath(cfg.Server.ContextPath),
	)
	router.RegisterRoutes(h, userHandler, appHandler, chatHistoryHandler, userService, redisClient)
	if err := taskManager.StartWorkers(); err != nil {
		panic(err)
	}
	h.OnShutdown = append(h.OnShutdown, func(ctx context.Context) {
		timeout := cfg.Task.ShutdownTimeout
		if timeout <= 0 {
			timeout = 20 * time.Second
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_ = taskManager.Shutdown(shutdownCtx)
	})
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
		ProvideHTMLAgent,
		ProvideMultiFileAgent,
		ProvideVueAgent,
		ProvideSummaryAgent,
		ProvideChatAgent,
		agent.NewCodeGenAgentFactory,
		ProvideSimpleWorkflow,
		ProvideMemoryStore,
		ProvideToolManager,
		ProvideTaskOptions,
		ProvideTaskManager,
	))
}
