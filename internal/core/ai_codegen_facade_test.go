package core

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/redis/go-redis/v9"
)

func TestGenCodeStreamAndSave(t *testing.T) {
	cfg := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(cfg)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	codeAgent := agent.NewCodeGenAgentFactory(chatModel, redisClient, enum.MultiFileGen)

	saver, err := saver.NewCodeSaver()
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
