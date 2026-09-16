package agent

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/llm"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
	"github.com/redis/go-redis/v9"
)

func TestCodeGenAgent_GenerateHtmlCode(t *testing.T) {
	if testing.Short() || os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("skipping integration test (set RUN_INTEGRATION=1 to enable)")
	}
	initConfig := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(initConfig)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", initConfig.Redis.Host, initConfig.Redis.Port),
		Password: initConfig.Redis.Password,
		DB:       initConfig.Redis.DB,
	})
	memoryStore := store.NewRedisMemoryStore(redisClient, fmt.Sprintf("test:%d", time.Now().UnixNano()), 20, 24*time.Hour)
	codeGenAgent := NewCodeGenAgent(chatModel, enum.HtmlCodeGen, memoryStore)
	code, err := codeGenAgent.GenerateHtmlCode(context.Background(), "test test test")
	if err != nil {
		panic(err)
	}
	assert.NotNil(t, code)
}

func TestCodeGenAgent_GenerateMultiFileCode(t *testing.T) {
	if testing.Short() || os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("skipping integration test (set RUN_INTEGRATION=1 to enable)")
	}
	initConfig := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(initConfig)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", initConfig.Redis.Host, initConfig.Redis.Port),
		Password: initConfig.Redis.Password,
		DB:       initConfig.Redis.DB,
	})
	memoryStore := store.NewRedisMemoryStore(redisClient, fmt.Sprintf("test:%d", time.Now().UnixNano()), 20, 24*time.Hour)
	codeGenAgent := NewCodeGenAgent(chatModel, enum.MultiFileGen, memoryStore)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	code, err := codeGenAgent.GenerateMultiFileCode(ctx, "this is a test")
	if err != nil {
		panic(err)
	}
	assert.NotNil(t, code)
}
