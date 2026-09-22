package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/myfile"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type fakeRoutingAgent struct {
	generationType enum.CodeGenTypeEnum
}

func (a *fakeRoutingAgent) RouteCodeGenType(context.Context, []*schema.Message) (enum.CodeGenTypeEnum, error) {
	return a.generationType, nil
}

type fakeCodeGenFacade struct {
	calls int
}

func (f *fakeCodeGenFacade) GenCodeStreamAndSave(context.Context, int64, []*schema.Message, enum.CodeGenTypeEnum) (*schema.StreamReader[*schema.Message], error) {
	f.calls++
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		writer.Send(&schema.Message{Content: "<main>Hello</main>"}, nil)
	}()
	return reader, nil
}

type fakeQualityAgent struct {
	results []bool
	index   int
}

func (a *fakeQualityAgent) CheckCodeQuality(context.Context, []*schema.Message) (structQualityResult, error) {
	result := a.results[a.index]
	if a.index < len(a.results)-1 {
		a.index++
	}
	return structQualityResult{IsValid: result}, nil
}

// structQualityResult keeps the fake node independent from LLM and filesystem code.
type structQualityResult struct {
	IsValid bool
}

func fakeRouterNode(agent *fakeRoutingAgent) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
		if input.WorkFlowContext == nil {
			input.WorkFlowContext = &state.WorkFlowContext{}
		}
		input.WorkFlowContext.GenerationType, _ = agent.RouteCodeGenType(ctx, nil)
		return input, nil
	})
}

func fakeCodeGeneratorNode(facade *fakeCodeGenFacade) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
		reader, err := facade.GenCodeStreamAndSave(ctx, 1, nil, input.WorkFlowContext.GenerationType)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		input.WorkFlowContext.GenerateCodeDir = "fake-generated"
		return input, nil
	})
}

func fakeQualityNode(agent *fakeQualityAgent) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
		result, _ := agent.CheckCodeQuality(ctx, nil)
		input.WorkFlowContext.QualityResult.IsValid = result.IsValid
		return input, nil
	})
}

func newFakeWorkflow(qualityResults []bool) (*SimpleWorkflow, *fakeCodeGenFacade) {
	facade := &fakeCodeGenFacade{}
	return NewSimpleWorkflow(
		fakeRouterNode(&fakeRoutingAgent{generationType: enum.HtmlCodeGen}),
		node.NewPromptEnhancerNode(),
		fakeCodeGeneratorNode(facade),
		fakeQualityNode(&fakeQualityAgent{results: qualityResults}),
	), facade
}

func TestSimpleWorkflow_FakeGraph(t *testing.T) {
	workflow, facade := newFakeWorkflow([]bool{true})
	result, err := workflow.Execute(context.Background(), "创建个人博客首页")

	require.NoError(t, err)
	require.Equal(t, enum.HtmlCodeGen, result.GenerationType)
	require.True(t, result.QualityResult.IsValid)
	require.Equal(t, 1, facade.calls)
}

func TestSimpleWorkflow_FakeGraph_QualityRetryLimit(t *testing.T) {
	workflow, facade := newFakeWorkflow([]bool{false, false, false, false})
	workflowContext := &state.WorkFlowContext{
		OriginalPrompt: "创建个人博客首页",
		GenerationType: enum.HtmlCodeGen,
		MaxRetries:     3,
	}
	runnable, err := workflow.CreateWorkflow(context.Background())
	require.NoError(t, err)

	_, err = runnable.Invoke(context.Background(), &state.GraphState{WorkFlowContext: workflowContext})
	require.Error(t, err)
	require.Equal(t, 4, facade.calls)
	require.Contains(t, err.Error(), "quality check failed after 3 retries")
}

func TestSimpleWorkflow_RealLLMGraph(t *testing.T) {
	if os.Getenv("RUN_REAL_LLM") != "1" {
		t.Skip("set RUN_REAL_LLM=1 to run the real LLM graph test")
	}

	cfg := config.InitConfig()
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

	routingAgent := agent.NewCodeGenTypeRoutingAgent(chatModel, adk.CheckPointStore(nil), metrics)
	qualityAgent := agent.NewCodeQualityCheckAgent(chatModel, adk.CheckPointStore(nil), metrics)
	htmlAgent := agent.NewHtmlCodeGenAgent(chatModel, adk.CheckPointStore(nil), metrics, nil)
	multiFileAgent := agent.NewMultiFileCodeGenAgent(chatModel, adk.CheckPointStore(nil), metrics, nil)
	vueAgent := agent.NewVueCodeGenAgent(chatModel, adk.CheckPointStore(nil), metrics, nil, nil)
	factory := agent.NewCodeGenAgentFactory(htmlAgent, multiFileAgent, vueAgent)
	codeSaver, err := saver.NewCodeSaver()
	require.NoError(t, err)
	facade := core.NewNoCodeAIGenFacade(factory, codeSaver)

	workflow := NewSimpleWorkflow(
		node.NewRouterNode(routingAgent),
		node.NewPromptEnhancerNode(),
		node.NewCodeGeneratorNode(facade),
		node.NewCodeQualityCheckNode(qualityAgent),
	)
	runnable, err := workflow.CreateWorkflow(context.Background())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	result, err := runnable.Invoke(ctx, &state.GraphState{
		WorkFlowContext: &state.WorkFlowContext{
			AppID:          time.Now().Unix(),
			OriginalPrompt: "创建一个简单的个人博客首页，包含标题、简介和文章列表",
			MaxRetries:     3,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.WorkFlowContext)
	require.NotEmpty(t, result.WorkFlowContext.GenerateCodeDir)
	require.NotEmpty(t, result.WorkFlowContext.QualityResult)
	require.FileExists(t, result.WorkFlowContext.GenerateCodeDir+"/index.html")
	t.Logf("real graph generated: %s", result.WorkFlowContext.GenerateCodeDir)
}

func TestSimpleWorkflow_RealLLMVUeGraph(t *testing.T) {
	if os.Getenv("RUN_REAL_LLM") != "1" {
		t.Skip("set RUN_REAL_LLM=1 to run the real Vue LLM graph test")
	}

	cfg := config.InitConfig()
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

	toolManager, err := aitools.NewToolManager()
	require.NoError(t, err)

	routingAgent := agent.NewCodeGenTypeRoutingAgent(chatModel, nil, metrics)
	qualityAgent := agent.NewCodeQualityCheckAgent(chatModel, nil, metrics)
	htmlAgent := agent.NewHtmlCodeGenAgent(chatModel, nil, metrics, nil)
	multiFileAgent := agent.NewMultiFileCodeGenAgent(chatModel, nil, metrics, nil)
	vueAgent := agent.NewVueCodeGenAgent(chatModel, nil, metrics, toolManager, nil)
	factory := agent.NewCodeGenAgentFactory(htmlAgent, multiFileAgent, vueAgent)
	codeSaver, err := saver.NewCodeSaver()
	require.NoError(t, err)
	facade := core.NewNoCodeAIGenFacade(factory, codeSaver)

	workflow := NewSimpleWorkflow(
		node.NewRouterNode(routingAgent),
		node.NewPromptEnhancerNode(),
		node.NewCodeGeneratorNode(facade),
		node.NewCodeQualityCheckNode(qualityAgent),
	)
	runnable, err := workflow.CreateWorkflow(context.Background())
	require.NoError(t, err)

	appID := time.Now().Unix()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ctx = context.WithValue(ctx, "appId", appID)
	result, err := runnable.Invoke(ctx, &state.GraphState{
		WorkFlowContext: &state.WorkFlowContext{
			AppID:          appID,
			OriginalPrompt: "请明确生成一个 Vue 3 项目，包含 package.json、src/main.js、src/App.vue 和可运行的首页按钮。必须使用文件工具写入项目文件。",
			MaxRetries:     3,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.WorkFlowContext)
	require.Equal(t, enum.VueCodeGen, result.WorkFlowContext.GenerationType)
	require.True(t, result.WorkFlowContext.QualityResult.IsValid)

	outputRoot, err := myfile.GetCodeOutputRoot()
	require.NoError(t, err)
	projectDir := filepath.Join(outputRoot, fmt.Sprintf("vue_project_%d", appID))
	require.DirExists(t, projectDir)
	require.FileExists(t, filepath.Join(projectDir, "package.json"))
	require.FileExists(t, filepath.Join(projectDir, "src", "App.vue"))
	t.Logf("real Vue graph generated: %s", projectDir)
}
