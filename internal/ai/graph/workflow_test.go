package graph

import (
	"context"
	"fmt"
	"testing"
	"workspace-yikou-ai-go/biz/core"
	"workspace-yikou-ai-go/biz/core/parser"
	"workspace-yikou-ai-go/biz/core/saver"
	"workspace-yikou-ai-go/biz/logic/chathistory"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/manager"
	"github.com/cloudwego/eino-ext/components/model/openai"
)

func TestWorkflow(t *testing.T) {
	fmt.Println("=== 简化版网站生成工作流 ===")
	if err := RunSimpleWorkflow(); err != nil {
		logger.Errorf("工作流执行失败: %v", err)
	}
}

func TestRunSimpleStateWorkflow(t *testing.T) {
	fmt.Println("=== 简化版网站State生成工作流 ===")
	if err := RunSimpleStateWorkflow(); err != nil {
		logger.Errorf("工作流执行失败: %v", err)
	}
}

func TestRunWorkflowApp(t *testing.T) {
	fmt.Println("=== 网站生成工作流 ===")
	if err := RunSimpleStateWorkflow(); err != nil {
		logger.Errorf("工作流执行失败: %v", err)
	}
}

func TestCodeGenWorkflow_ExecuteWorkflow(t *testing.T) {
	initConfig := config.InitConfig()
	chatModel := llm.NewBaseAiChatModel(initConfig)
	reasoningChatModel := llm.NewReasoningChatModel(initConfig)
	node.InitImageCollectorNode(initConfig, chatModel)
	node.InitRouterNode(chatModel)
	redis := dal.InitRedis(initConfig)
	db := dal.InitDB(initConfig)
	summaryAgentFactory := agent.NewChatSummaryAgentFactory(chatModel)
	chatHistoryService := chathistory.NewChatHistoryService(db, summaryAgentFactory)
	toolManager, err := aitools.NewToolManager()
	if err != nil {
		fmt.Println(err)
	}
	codeGenAgentFactory := agent.NewCodeGenAgentFactory(chatModel, reasoningChatModel, redis, chatHistoryService, toolManager)
	node.InitCodeGeneratorNode(core.NewYiKouAiCodegenFacade(ai.NewYiKouAiCodegenService((*openai.ChatModel)(chatModel)),
		parser.NewCodeParserExecutor(),
		saver.NewCodeFileSaverExecutor(),
		codeGenAgentFactory))
	_, err = ExecuteWorkflow(context.Background(), "创建一个Vue前端项目，包含用户管理和数据展示功能")
	if err != nil {
		fmt.Println(err)
	}
}

func TestCodeGenWorkflow_ExecuteWorkflow2(t *testing.T) {
	initConfig := config.InitConfig()
	chatModel := llm.NewBaseAiChatModel(initConfig)
	reasoningChatModel := llm.NewReasoningChatModel(initConfig)
	redis := dal.InitRedis(initConfig)
	db := dal.InitDB(initConfig)
	client := dal.InitCOSClient(initConfig)
	cosManager := manager.NewCosManager(client, initConfig)
	node.InitImagePlanNode(chatModel)
	node.InitContentImageCollectorNode(initConfig)
	node.InitDiagramCollectorNode(cosManager)
	node.InitLogoCollectorNode(initConfig, cosManager)
	node.InitRouterNode(chatModel)
	summaryAgentFactory := agent.NewChatSummaryAgentFactory(chatModel)
	chatHistoryService := chathistory.NewChatHistoryService(db, summaryAgentFactory)
	toolManager, err := aitools.NewToolManager()
	if err != nil {
		fmt.Println(err)
	}
	codeGenAgentFactory := agent.NewCodeGenAgentFactory(chatModel, reasoningChatModel, redis, chatHistoryService, toolManager)
	node.InitCodeGeneratorNode(core.NewYiKouAiCodegenFacade(ai.NewYiKouAiCodegenService((*openai.ChatModel)(chatModel)),
		parser.NewCodeParserExecutor(),
		saver.NewCodeFileSaverExecutor(),
		codeGenAgentFactory))
	_, err = ExecuteWorkflow(context.Background(), "创建一个简单的个人主页")
	if err != nil {
		fmt.Println(err)
	}
}
