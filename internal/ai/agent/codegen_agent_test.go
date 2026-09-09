package agent

import (
	"context"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
)

func TestCodeGenAgent_GenerateHtmlCode(t *testing.T) {
	initConfig := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(initConfig)
	codeGenAgent := NewCodeGenAgent(chatModel, enum.HtmlCodeGen)
	code, err := codeGenAgent.GenerateHtmlCode(context.Background(), "test test test")
	if err != nil {
		panic(err)
	}
	assert.NotNil(t, code)
}

func TestCodeGenAgent_GenerateMultiFileCode(t *testing.T) {
	initConfig := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(initConfig)
	codeGenAgent := NewCodeGenAgent(chatModel, enum.MultiFileGen)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	code, err := codeGenAgent.GenerateMultiFileCode(ctx, "this is a test")
	if err != nil {
		panic(err)
	}
	assert.NotNil(t, code)
}
