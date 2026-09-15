package agent

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/ai_model"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type CodeGenAgent struct {
	*BaseAgent
	agentType enum.CodeGenTypeEnum
}

func NewCodeGenAgent(chatModel ChatModelWrapperAdaptor, codeGenType enum.CodeGenTypeEnum, memoryStore *store.RedisMemoryStore) *CodeGenAgent {
	baseAgent := NewBaseAgent(chatModel, memoryStore)
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

func (a *CodeGenAgent) GenerateHtmlCode(ctx context.Context, userMessage string) (*aimodel.HtmlCodeResponse, error) {
	chatTemplate, err := prompt.NewHtmlChatTemplate()
	if err != nil {
		return nil, err
	}

	adkAgent := a.getAdkAgent()
	message, err := a.Generate(ctx, userMessage,
		chatTemplate, adkAgent)
	if err != nil {
		return nil, err
	}
	var result aimodel.HtmlCodeResponse
	parsedContent := ParseCodeResponse([]byte(message.Content))
	err = json.Unmarshal([]byte(parsedContent), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *CodeGenAgent) GenerateMultiFileCode(ctx context.Context, userMessage string) (*aimodel.MultiFileCodeResponse, error) {
	chatTemplate, err := prompt.NewMultiFileChatTemplate()
	if err != nil {
		return nil, err
	}

	adkAgent := a.getAdkAgent()
	message, err := a.Generate(ctx, userMessage,
		chatTemplate, adkAgent)
	if err != nil {
		return nil, err
	}
	var result aimodel.MultiFileCodeResponse
	parsedContent := ParseCodeResponse([]byte(message.Content))
	err = json.Unmarshal([]byte(parsedContent), &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *CodeGenAgent) GenerateHtmlCodeStream(ctx context.Context, userMessage string) (*schema.StreamReader[*schema.Message], error) {
	chatTemplate, err := prompt.NewHtmlChatTemplate()
	if err != nil {
		return nil, err
	}

	adkAgent := a.getAdkAgent()
	return a.GenerateStream(ctx, userMessage, chatTemplate, adkAgent)
}

func (a *CodeGenAgent) GenerateMultiFileCodeStream(ctx context.Context, userMessage string) (*schema.StreamReader[*schema.Message], error) {
	chatTemplate, err := prompt.NewMultiFileChatTemplate()
	if err != nil {
		return nil, err
	}

	adkAgent := a.getAdkAgent()
	return a.GenerateStream(ctx, userMessage, chatTemplate, adkAgent)
}
func (a *CodeGenAgent) newMultiFileCodeGenAgent() *adk.ChatModelAgent {
	if err := prompt.LoadPrompts(); err != nil {
		logger.Errorf("loading prompts failed: %v", err)
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
		logger.Errorf("loading prompts failed: %v", err)
		return nil
	}
	return a.NewAdkAgent(
		"HtmlFileCodeGenAgent",
		"A html code generator with strong code generation capabilities",
		prompt.GetHtmlPrompt(),
		[]*tool.BaseTool{},
	)
}

func ParseCodeResponse(raw []byte) []byte {
	cleaned := bytes.TrimSpace(raw)

	if bytes.HasPrefix(cleaned, []byte("```json")) {
		cleaned = bytes.TrimPrefix(cleaned, []byte("```json"))
	} else if bytes.HasPrefix(cleaned, []byte("```")) {
		cleaned = bytes.TrimPrefix(cleaned, []byte("```"))
	}

	if bytes.HasSuffix(cleaned, []byte("```")) {
		cleaned = bytes.TrimSuffix(cleaned, []byte("```"))
	}

	cleaned = bytes.TrimSpace(cleaned)

	return cleaned
}
