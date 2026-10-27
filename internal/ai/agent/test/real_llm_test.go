package agent_test

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/schema"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestRealLLMAgents(t *testing.T) {
	if os.Getenv("RUN_REAL_LLM") != "1" {
		t.Skip("set RUN_REAL_LLM=1 to run real LLM tests")
	}

	cfg := config.InitConfig()
	require.NotNil(t, cfg)
	require.NotEmpty(t, cfg.AI.Provider)
	require.NotEmpty(t, cfg.AI.Model)
	require.NotEmpty(t, cfg.AI.APIKey)

	var chatModel agent.ChatModelWrapperAdaptor
	switch cfg.AI.Provider {
	case "openai":
		chatModel = llm.NewOpenAIChatModel(cfg)
	case "claude":
		chatModel = llm.NewClaudeChatModel(cfg)
	default:
		t.Fatalf("unsupported configured AI provider: %s", cfg.AI.Provider)
	}

	metrics := monitor.NewAiModelMetricsCollector(prometheus.NewRegistry())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("routing", func(t *testing.T) {
		a := agent.NewCodeGenTypeRoutingAgent(chatModel, nil, metrics)
		got, err := a.RouteCodeGenType(ctx, []*schema.Message{
			{Role: schema.User, Content: "创建一个简单的个人博客首页，包含文章列表和个人介绍"},
		})
		require.NoError(t, err)
		require.NotEmpty(t, got)
		t.Logf("routing result: %s", got)
	})

	t.Run("quality check", func(t *testing.T) {
		a := agent.NewCodeQualityCheckAgent(chatModel, nil, metrics)
		got, err := a.CheckCodeQuality(ctx, []*schema.Message{
			{Role: schema.User, Content: `请检查以下 HTML：<html><body><h1>Hello</h1></body></html>`},
		})
		require.NoError(t, err)
		t.Logf("quality result: valid=%v errors=%v suggestions=%v", got.IsValid, got.Errors, got.Suggestions)
	})

	t.Run("html generation", func(t *testing.T) {
		a := agent.NewHtmlCodeGenAgent(chatModel, nil, metrics, nil)
		got, err := a.GenerateHtmlCode(ctx, []*schema.Message{
			{Role: schema.User, Content: "生成一个只有 HTML 和 CSS 的个人博客首页，返回约定的 JSON 结构"},
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotEmpty(t, got.HtmlCode)
		t.Logf("html length: %d", len(got.HtmlCode))
	})

	t.Run("multi file generation", func(t *testing.T) {
		a := agent.NewMultiFileCodeGenAgent(chatModel, nil, metrics, nil)
		got, err := a.GenerateMultiFileCode(ctx, []*schema.Message{
			{Role: schema.User, Content: "生成一个简单的多文件网页，返回 html_code、css_code、js_code JSON 字段"},
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotEmpty(t, got.HtmlCode)
		t.Logf("multi-file lengths: html=%d css=%d js=%d", len(got.HtmlCode), len(got.CssCode), len(got.JsCode))
	})

	t.Run("chat summary", func(t *testing.T) {
		a := agent.NewChatSummaryAgent(chatModel, nil, metrics)
		got, err := a.SummarizeChat(ctx, []*schema.Message{
			{Role: schema.User, Content: "用户需要一个极简风格的个人博客，重点展示文章列表。"},
			{Role: schema.Assistant, Content: "建议使用白色背景、深色文字和卡片式文章列表。"},
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NotEmpty(t, got.Content)
		t.Logf("summary: %s", got.Content)
	})

	t.Run("image collection plan", func(t *testing.T) {
		a := agent.NewImageCollectionPlanAgent(chatModel, nil, metrics)
		got, err := a.PlanImageCollection(ctx, []*schema.Message{
			{Role: schema.User, Content: "规划一个旅行博客需要哪些图片素材，只返回约定 JSON"},
		})
		require.NoError(t, err)
		t.Logf("image plan counts: content=%d illustration=%d diagram=%d logo=%d",
			len(got.ContentImageTasks),
			len(got.IllustrationTasks),
			len(got.DiagramTasks),
			len(got.LogoTasks))
	})

	t.Run("vue generation stream", func(t *testing.T) {
		a := agent.NewVueCodeGenAgent(chatModel, nil, metrics, nil, nil)
		reader, err := a.GenerateVueProjectCodeStream(ctx, []*schema.Message{
			{Role: schema.User, Content: "生成一个最小可运行的 Vue 3 项目，包含首页和一个简单按钮"},
		})
		require.NoError(t, err)
		defer reader.Close()

		var content string
		for {
			message, recvErr := reader.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			require.NoError(t, recvErr)
			if message != nil {
				content += message.Content
			}
		}
		require.NotEmpty(t, content)
		t.Logf("vue stream length: %d", len(content))
	})
}
