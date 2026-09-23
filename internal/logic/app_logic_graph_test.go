package logic

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/workflow"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestBuildWorkflowPromptFake(t *testing.T) {
	history := []*schema.Message{
		schema.UserMessage("创建首页"),
		schema.AssistantMessage("已生成首页", nil),
	}

	prompt := buildWorkflowPrompt(history, "src/App.vue\nsrc/main.js", "增加登录按钮")

	require.Contains(t, prompt, "src/App.vue")
	require.Contains(t, prompt, "创建首页")
	require.Contains(t, prompt, "已生成首页")
	require.Contains(t, prompt, "增加登录按钮")
}

type fakeMemoryStore struct {
	mu       sync.Mutex
	messages map[string][]*schema.Message
}

func newFakeMemoryStore() *fakeMemoryStore {
	return &fakeMemoryStore{messages: make(map[string][]*schema.Message)}
}

func (s *fakeMemoryStore) GetMessages(_ context.Context, memoryID string) ([]*schema.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*schema.Message(nil), s.messages[memoryID]...), nil
}

func (s *fakeMemoryStore) AppendMessage(_ context.Context, message *schema.Message, memoryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[memoryID] = append(s.messages[memoryID], message)
	return nil
}

func (s *fakeMemoryStore) ClearMessages(_ context.Context, memoryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.messages, memoryID)
	return nil
}

func (s *fakeMemoryStore) AddAssistantMessage(ctx context.Context, message, memoryID string) error {
	return s.AppendMessage(ctx, schema.AssistantMessage(message, nil), memoryID)
}

func (s *fakeMemoryStore) AddUserMessage(ctx context.Context, message, memoryID string) error {
	return s.AppendMessage(ctx, schema.UserMessage(message), memoryID)
}

func TestGraphToGenCodeRealLLM(t *testing.T) {
	if os.Getenv("RUN_REAL_LLM") != "1" {
		t.Skip("set RUN_REAL_LLM=1 to run the real GraphToGenCode integration test")
	}

	cfg := config.InitConfig()
	userID := int64(101)
	appID := int64(202)

	memoryStore := newFakeMemoryStore()
	memoryID := "202"
	require.NoError(t, memoryStore.AddUserMessage(context.Background(), "之前创建了首页", memoryID))
	require.NoError(t, memoryStore.AddAssistantMessage(context.Background(), "首页已经生成", memoryID))
	metrics := monitor.NewAiModelMetricsCollector(prometheus.NewRegistry())

	var chatModel agent.ChatModelWrapperAdaptor
	switch cfg.AI.Provider {
	case "openai":
		chatModel = llm.NewOpenAIChatModel(cfg)
	case "claude":
		chatModel = llm.NewClaudeChatModel(cfg)
	default:
		t.Fatalf("unsupported AI provider: %s", cfg.AI.Provider)
	}

	qualityAgent := agent.NewCodeQualityCheckAgent(chatModel, adk.CheckPointStore(nil), metrics)
	routingAgent := agent.NewCodeGenTypeRoutingAgent(chatModel, adk.CheckPointStore(nil), metrics)
	htmlAgent := agent.NewHtmlCodeGenAgent(chatModel, adk.CheckPointStore(nil), metrics, nil)
	multiFileAgent := agent.NewMultiFileCodeGenAgent(chatModel, adk.CheckPointStore(nil), metrics, nil)
	vueAgent := agent.NewVueCodeGenAgent(chatModel, adk.CheckPointStore(nil), metrics, nil, nil)
	factory := agent.NewCodeGenAgentFactory(htmlAgent, multiFileAgent, vueAgent)
	codeSaver, err := saver.NewCodeSaver()
	require.NoError(t, err)
	facade := core.NewNoCodeAIGenFacade(factory, codeSaver)
	graph := workflow.NewSimpleWorkflow(
		node.NewRouterNode(routingAgent),
		node.NewPromptEnhancerNode(),
		node.NewCodeGeneratorNode(facade),
		node.NewCodeQualityCheckNode(qualityAgent),
	)

	chatHistory := &fakeChatHistoryService{}
	appService := NewAppService(facade, nil, chatHistory, nil, memoryStore, graph)
	appService.loadApp = func(context.Context, int64) (*model.App, error) {
		return &model.App{
			ID: appID, UserID: userID, CodeGenType: string(enum.HtmlCodeGen),
			ProjectArchitecture: "src/App.vue\nsrc/main.js",
		}, nil
	}
	var updatedArchitecture string
	appService.updateArchitecture = func(_ context.Context, _ int64, architecture string) error {
		updatedArchitecture = architecture
		return nil
	}
	user := &api.UserVo{ID: userID}

	stream, workflowContext, err := appService.GraphToGenCode(
		context.Background(),
		appID,
		"请增加一个登录按钮",
		user,
	)
	require.NoError(t, err)
	require.Contains(t, workflowContext.OriginalPrompt, "之前创建了首页")
	require.Contains(t, workflowContext.OriginalPrompt, "src/App.vue")
	require.Contains(t, workflowContext.OriginalPrompt, "请增加一个登录按钮")
	defer stream.Close()

	for {
		_, err = stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}
	require.NotEmpty(t, workflowContext.CodeContent)
	require.NotEmpty(t, memoryStore.messages[memoryID])
	require.NotEmpty(t, updatedArchitecture)
	require.Len(t, chatHistory.messages, 2)
}

var _ store.MemoryStore = (*fakeMemoryStore)(nil)

type fakeChatHistoryService struct {
	mu       sync.Mutex
	messages []string
}

func (s *fakeChatHistoryService) ListAppChatHistoryByCursor(context.Context, int64, int32, time.Time, int64, *api.UserVo) (*api.CursorResponse, error) {
	return &api.CursorResponse{}, nil
}

func (s *fakeChatHistoryService) AddChatMessage(_ context.Context, _ int64, message string, _ enum.ChatHistoryMessageTypeEnum, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return nil
}

func (s *fakeChatHistoryService) DeleteByAppId(context.Context, int64) error {
	return nil
}

func (s *fakeChatHistoryService) ListAppChatHistoryByPage(context.Context, int64, int32, time.Time, int64, *api.UserVo) (*response.PageResponse[*model.ChatHistory], error) {
	return nil, nil
}

func (s *fakeChatHistoryService) ListAllChatHistoryByPageForAdmin(context.Context, int32, int32, *api.NoCodeChatHistoryQueryRequest) (*response.PageResponse[*model.ChatHistory], error) {
	return nil, nil
}

func (s *fakeChatHistoryService) LoadChatHistoryToMemory(context.Context, int64, store.MemoryStore, int) (int, error) {
	return 0, nil
}
