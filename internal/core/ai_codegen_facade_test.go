package core

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
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
)

func TestGenCodeStreamAndSave(t *testing.T) {
	cfg := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(cfg)
	codeAgent := agent.NewCodeGenAgent(chatModel, enum.HtmlCodeGen)
	saver, err := saver.NewCodeSaver("gen_code_test")
	if err != nil {
		panic(err)
	}
	f := NewNoCodeAIGenFacade(codeAgent, saver)
	reader, err := f.GenCodeStreamAndSave(context.Background(), 1, "做一个简单的网页", enum.MultiFileGen)
	if err != nil {
		panic(err)
	}
	defer reader.Close()
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			panic(err)
		}

		if chunk != nil && chunk.Content != "" {
			// 直接写入 os.Stdout 保证打字机效果立即输出
			os.Stdout.WriteString(chunk.Content)
		}
	}
	time.Sleep(2 * time.Second)
	assert.Nil(t, err)
}
