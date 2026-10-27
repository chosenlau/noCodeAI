package agent_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type fakeChatModel struct {
	mu        sync.Mutex
	responses []fakeResponse
}

type fakeResponse struct {
	message *schema.Message
	err     error
}

func (m *fakeChatModel) Generate(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.responses) == 0 {
		return nil, errors.New("fake model has no response")
	}
	response := m.responses[0]
	m.responses = m.responses[1:]
	return response.message, response.err
}

func (m *fakeChatModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer writer.Close()
		if message == nil {
			return
		}
		for _, chunk := range []string{"stream-", message.Content} {
			writer.Send(&schema.Message{Content: chunk}, nil)
		}
	}()
	return reader, nil
}

func (m *fakeChatModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

type fakeWrapper struct {
	model model.ToolCallingChatModel
}

func (w fakeWrapper) GetChatModel() model.ToolCallingChatModel {
	return w.model
}

func (w fakeWrapper) GetModelName() string {
	return "agent-test-model"
}

func newWrapper(responses ...fakeResponse) fakeWrapper {
	return fakeWrapper{model: &fakeChatModel{responses: responses}}
}

func textResponse(content string) fakeResponse {
	return fakeResponse{message: &schema.Message{Content: content}}
}

func errorResponse(err error) fakeResponse {
	return fakeResponse{err: err}
}

func testMetrics() *monitor.AiModelMetricsCollector {
	return monitor.NewAiModelMetricsCollector(prometheus.NewRegistry())
}

func TestAgents_WithFakeModel(t *testing.T) {
	t.Run("routing", func(t *testing.T) {
		a := agent.NewCodeGenTypeRoutingAgent(
			newWrapper(textResponse("html")),
			nil,
			testMetrics(),
		)
		got, err := a.RouteCodeGenType(context.Background(), []*schema.Message{
			{Role: schema.User, Content: "create a landing page"},
		})
		require.NoError(t, err)
		require.Equal(t, "html", string(got))
	})

	t.Run("quality check valid json", func(t *testing.T) {
		a := agent.NewCodeQualityCheckAgent(
			newWrapper(textResponse(`{"isValid":true,"errors":[],"suggestions":[]}`)),
			nil,
			testMetrics(),
		)
		got, err := a.CheckCodeQuality(context.Background(), nil)
		require.NoError(t, err)
		require.True(t, got.IsValid)
		require.Empty(t, got.Errors)
	})

	t.Run("quality check malformed json fails closed", func(t *testing.T) {
		a := agent.NewCodeQualityCheckAgent(
			newWrapper(textResponse("not-json")),
			nil,
			testMetrics(),
		)
		got, err := a.CheckCodeQuality(context.Background(), nil)
		require.Error(t, err)
		require.False(t, got.IsValid)
	})

	t.Run("html code generation", func(t *testing.T) {
		a := agent.NewHtmlCodeGenAgent(
			newWrapper(textResponse("```json\n{\"html_code\":\"<main>Hello</main>\",\"description\":\"demo\"}\n```")),
			nil,
			testMetrics(),
			nil,
		)
		got, err := a.GenerateHtmlCode(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, "<main>Hello</main>", got.HtmlCode)
		require.Equal(t, "demo", got.Description)
	})

	t.Run("multi file code generation", func(t *testing.T) {
		a := agent.NewMultiFileCodeGenAgent(
			newWrapper(textResponse(`{"html_code":"<main/>","description":"demo","js_code":"console.log(1)","css_code":"main{color:red}"}`)),
			nil,
			testMetrics(),
			nil,
		)
		got, err := a.GenerateMultiFileCode(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, "<main/>", got.HtmlCode)
		require.Equal(t, "console.log(1)", got.JsCode)
		require.Equal(t, "main{color:red}", got.CssCode)
	})

	t.Run("chat summary", func(t *testing.T) {
		a := agent.NewChatSummaryAgent(
			newWrapper(textResponse("summary")),
			nil,
			testMetrics(),
		)
		got, err := a.SummarizeChat(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, "summary", got.Content)
	})

	t.Run("image collection plan", func(t *testing.T) {
		a := agent.NewImageCollectionPlanAgent(
			newWrapper(textResponse("```json\n{\"contentImageTasks\":[{\"query\":\"city\"}],\"illustrationTasks\":[],\"diagramTasks\":[],\"logoTasks\":[]}\n```")),
			nil,
			testMetrics(),
		)
		got, err := a.PlanImageCollection(context.Background(), nil)
		require.NoError(t, err)
		require.Len(t, got.ContentImageTasks, 1)
		require.Equal(t, "city", got.ContentImageTasks[0].Query)
	})

	t.Run("image collection", func(t *testing.T) {
		a := agent.NewImageCollectionAgent(
			newWrapper(textResponse("images collected")),
			nil,
			testMetrics(),
			nil,
			nil,
			nil,
			nil,
		)
		got, err := a.CollectImages(context.Background(), nil)
		require.NoError(t, err)
		require.Equal(t, "images collected", got.Content)
	})
}

func TestAgents_ErrorAndStreamPaths(t *testing.T) {
	t.Run("model error is returned", func(t *testing.T) {
		a := agent.NewChatSummaryAgent(
			newWrapper(
				errorResponse(errors.New("model unavailable")),
				errorResponse(errors.New("model unavailable")),
				errorResponse(errors.New("model unavailable")),
				errorResponse(errors.New("model unavailable")),
			),
			nil,
			testMetrics(),
		)
		_, err := a.SummarizeChat(context.Background(), nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "model unavailable")
	})

	t.Run("stream output is readable", func(t *testing.T) {
		a := agent.NewHtmlCodeGenAgent(
			newWrapper(textResponse(`{"html_code":"<main/>"}`)),
			nil,
			testMetrics(),
			nil,
		)
		reader, err := a.GenerateHtmlCodeStream(context.Background(), nil)
		require.NoError(t, err)
		defer reader.Close()

		var content string
		for {
			message, recvErr := reader.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			require.NoError(t, recvErr)
			content += message.Content
		}
		require.Contains(t, content, "stream-")
		require.Contains(t, content, `"html_code":"<main/>"`)
	})
}

func TestAgents_CheckpointRequiresContextID(t *testing.T) {
	a := agent.NewChatSummaryAgent(
		newWrapper(textResponse("summary")),
		nilCheckpointStore{},
		testMetrics(),
	)
	_, err := a.SummarizeChat(context.Background(), nil)
	require.EqualError(t, err, "checkpointID not found in context")
}

type nilCheckpointStore struct{}

func (nilCheckpointStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, errors.New("not implemented")
}

func (nilCheckpointStore) Set(ctx context.Context, key string, value []byte) error {
	return errors.New("not implemented")
}

func (nilCheckpointStore) Delete(ctx context.Context, key string) error {
	return errors.New("not implemented")
}

var _ = aimodel.QualityResult{}
