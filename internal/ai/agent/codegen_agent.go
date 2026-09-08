package agent

import (
	"context"
	"encoding/json"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/model"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

type CodeGenAgent struct {
	*BaseAgent
	agentType enum.CodeGenTypeEnum
}

func NewCodeGenAgent(chatModel ChatModelWrapperAdaptor, codeGenType enum.CodeGenTypeEnum) *CodeGenAgent {
	baseAgent := NewBaseAgent(chatModel)
	return &CodeGenAgent{
		BaseAgent: baseAgent,
		agentType: codeGenType,
	}
}

func (a *CodeGenAgent) getAdkAgent() *adk.ChatModelAgent {
	switch a.agentType {
	case enum.HtmlCodeGen:
		return a.newHtmlFileCodeGenAgent()
	case enum.MultiFileGen:
		return a.newMultiFileCodeGenAgent()
	default:
		return nil
	}
}

func (a *CodeGenAgent) GenerateHtmlCode(ctx context.Context, userMessage string) (*model.HtmlCodeResponse, error) {
	chatTemplate, err := prompt.NewHtmlChatTemplate()
	if err != nil {
		return nil, err
	}

	adkAgent := a.getAdkAgent()
	message, err := a.Generate(ctx, userMessage+
		`You must answer strictly in the following JSON format:
		{
		  "htmlCode": "your html code here",
		  "description": "description of the code"
		}
		IMPORTANT: You must answer ONLY with a valid JSON object, no markdown, no code blocks, no backticks.
		`,
		chatTemplate, adkAgent)
	if err != nil {
		return nil, err
	}
	var result model.HtmlCodeResponse
	err = json.Unmarshal([]byte(message.Content), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *CodeGenAgent) GenerateMultiFileCode(ctx context.Context, userMessage string) (*model.MultiFileCodeResponse, error) {
	chatTemplate, err := prompt.NewMultiFileChatTemplate()
	if err != nil {
		return nil, err
	}

	adkAgent := a.getAdkAgent()
	message, err := a.Generate(ctx, userMessage+
		`You must answer strictly in the following JSON format:
		{
		  "htmlCode": "your html code here",
		  "description": "description of the code",
		  "cssCode": "your css code here",
		  "jsCode": "your javascript code here"
		}
		IMPORTANT: You must answer ONLY with a valid JSON object, no markdown, no code blocks, no backticks.
		`,
		chatTemplate, adkAgent)
	if err != nil {
		return nil, err
	}
	var result model.MultiFileCodeResponse
	err = json.Unmarshal([]byte(message.Content), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *CodeGenAgent) newMultiFileCodeGenAgent() *adk.ChatModelAgent {
	if err := prompt.LoadPrompts(); err != nil {
		logger.Errorf("加载prompts失败: %v", err)
		return nil
	}
	return a.NewAdkAgent(
		"MultiFileCodeGenAgent",
		"A multifile code generator with strong code generation capabilities",
		prompt.GetMultiFilePrompt(),
		[]*tool.BaseTool{},
	)
}

func (a *CodeGenAgent) newHtmlFileCodeGenAgent() *adk.ChatModelAgent {
	if err := prompt.LoadPrompts(); err != nil {
		logger.Errorf("加载prompts失败: %v", err)
		return nil
	}
	return a.NewAdkAgent(
		"HtmlFileCodeGenAgent",
		"A html code generator with strong code generation capabilities",
		prompt.GetHtmlPrompt(),
		[]*tool.BaseTool{},
	)
}
