//go:build external_integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/cloudwego/eino/schema"
	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/logic"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/redis/go-redis/v9"
)

// TestFacadeIntegration_MultiFile 测试完整的多文件生成流程
func TestFacadeIntegration_MultiFile(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	logger.Info("========== 开始集成测试：多文件代码生成 ==========")

	// 1. 初始化配置
	cfg := config.InitConfig()
	logger.Infof("✓ 配置加载成功: Provider=%s, Model=%s", cfg.AI.Provider, cfg.AI.Model)

	// 2. 初始化数据库
	db := dal.InitDB(cfg)
	logger.Info("✓ 数据库连接成功")

	// 3. 初始化 Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// 测试 Redis 连接
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("Redis 连接失败: %v", err)
	}
	logger.Info("✓ Redis 连接成功")

	// 4. 初始化 LLM 模型
	var chatModel agent.ChatModelWrapperAdaptor
	switch cfg.AI.Provider {
	case "claude":
		chatModel = llm.NewClaudeChatModel(cfg)
		logger.Info("✓ 使用 Claude 模型")
	case "openai":
		chatModel = llm.NewOpenAIChatModel(cfg)
		logger.Info("✓ 使用 OpenAI 模型")
	default:
		t.Fatalf("不支持的 AI Provider: %s", cfg.AI.Provider)
	}

	// 5. 初始化服务层（直接使用 logic 层实现）
	memory := store.NewRedisMemoryStore(redisClient, 20, 24*time.Hour)
	chatHistoryService := logic.NewChatHistoryService(db, memory, agent.NewChatSummaryAgent(chatModel, nil, nil))
	_ = chatHistoryService
	logger.Info("✓ ChatHistoryService 初始化成功")

	// 6. 初始化 CodeGenAgentFactory
	codeGenFactory := agent.NewCodeGenAgentFactory(agent.NewHtmlCodeGenAgent(chatModel, nil, nil, nil), agent.NewMultiFileCodeGenAgent(chatModel, nil, nil, nil), agent.NewVueCodeGenAgent(chatModel, nil, nil, nil, nil))
	logger.Info("✓ CodeGenAgentFactory 初始化成功")

	// 7. 初始化 CodeSaver
	codeSaver, err := saver.NewCodeSaver()
	if err != nil {
		t.Fatalf("CodeSaver 初始化失败: %v", err)
	}
	logger.Info("✓ CodeSaver 初始化成功")

	// 8. 创建 Facade
	facade := core.NewNoCodeAIGenFacade(codeGenFactory, codeSaver)
	logger.Info("✓ NoCodeAIGenFacade 初始化成功")

	// 9. 测试生成代码
	testAppID := int64(1)
	testPrompt := "创建一个简单的计数器网页，有加减按钮和显示数字"
	logger.Infof("开始生成代码: AppID=%d, Prompt=%s, Type=%s", testAppID, testPrompt, enum.MultiFileGen)

	startTime := time.Now()
	reader, err := facade.GenCodeStreamAndSave(ctx, testAppID, []*schema.Message{schema.UserMessage(testPrompt)}, enum.MultiFileGen)
	if err != nil {
		t.Fatalf("生成代码失败: %v", err)
	}
	defer reader.Close()

	// 10. 读取流式响应
	var contentBuilder string
	chunkCount := 0
	logger.Info("开始接收流式响应...")

	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			logger.Info("✓ 流式响应结束")
			break
		}
		if err != nil {
			t.Fatalf("接收流数据失败: %v", err)
		}

		if chunk != nil && chunk.Content != "" {
			contentBuilder += chunk.Content
			chunkCount++
			// 输出到控制台
			os.Stdout.WriteString(chunk.Content)
		}
	}

	elapsed := time.Since(startTime)
	logger.Infof("✓ 接收完成: 耗时 %v, 接收 %d 个数据块, 总长度 %d 字符",
		elapsed, chunkCount, len(contentBuilder))

	// 11. 等待后台保存完成
	logger.Info("等待后台文件保存完成...")
	time.Sleep(3 * time.Second)

	// 12. 验证结果
	if len(contentBuilder) == 0 {
		t.Error("❌ 生成的内容为空")
	} else {
		logger.Infof("✓ 生成内容长度: %d 字符", len(contentBuilder))
	}

	// 13. 检查保存的文件
	savePath := fmt.Sprintf("C:\\Users\\38917\\Desktop\\go_note\\noCodeAgent\\saves\\%s_%d",
		enum.MultiFileGen, testAppID)
	if _, err := os.Stat(savePath); os.IsNotExist(err) {
		t.Errorf("❌ 保存目录不存在: %s", savePath)
	} else {
		logger.Infof("✓ 文件已保存到: %s", savePath)

		// 检查必需文件
		requiredFiles := []string{"index.html", "style.css", "script.js", "descrition.md"}
		for _, filename := range requiredFiles {
			filePath := fmt.Sprintf("%s\\%s", savePath, filename)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("❌ 缺少文件: %s", filename)
			} else {
				logger.Infof("  ✓ %s 存在", filename)
			}
		}
	}

	logger.Info("========== 集成测试完成 ==========")
}

// TestFacadeIntegration_Html 测试 HTML 单文件生成流程
func TestFacadeIntegration_Html(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	logger.Info("========== 开始集成测试：HTML 单文件生成 ==========")

	cfg := config.InitConfig()
	db := dal.InitDB(cfg)

	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	var chatModel agent.ChatModelWrapperAdaptor
	if cfg.AI.Provider == "claude" {
		chatModel = llm.NewClaudeChatModel(cfg)
	} else {
		chatModel = llm.NewOpenAIChatModel(cfg)
	}

	memory := store.NewRedisMemoryStore(redisClient, 20, 24*time.Hour)
	chatHistoryService := logic.NewChatHistoryService(db, memory, agent.NewChatSummaryAgent(chatModel, nil, nil))
	_ = chatHistoryService
	codeGenFactory := agent.NewCodeGenAgentFactory(agent.NewHtmlCodeGenAgent(chatModel, nil, nil, nil), agent.NewMultiFileCodeGenAgent(chatModel, nil, nil, nil), agent.NewVueCodeGenAgent(chatModel, nil, nil, nil, nil))

	codeSaver, err := saver.NewCodeSaver()
	if err != nil {
		t.Fatalf("CodeSaver 初始化失败: %v", err)
	}

	facade := core.NewNoCodeAIGenFacade(codeGenFactory, codeSaver)

	testAppID := int64(2)
	testPrompt := "创建一个简单的问候页面，显示 Hello World"

	ctx := context.Background()
	reader, err := facade.GenCodeStreamAndSave(ctx, testAppID, []*schema.Message{schema.UserMessage(testPrompt)}, enum.HtmlCodeGen)
	if err != nil {
		t.Fatalf("生成 HTML 代码失败: %v", err)
	}
	defer reader.Close()

	var content string
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("接收流数据失败: %v", err)
		}
		if chunk != nil && chunk.Content != "" {
			content += chunk.Content
			os.Stdout.WriteString(chunk.Content)
		}
	}

	time.Sleep(3 * time.Second)

	if len(content) == 0 {
		t.Error("❌ 生成的内容为空")
	} else {
		logger.Infof("✓ HTML 生成成功，长度: %d", len(content))
	}

	logger.Info("========== HTML 集成测试完成 ==========")
}
