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
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
	"github.com/redis/go-redis/v9"
)

type nopChatHistoryService struct{}

func (nopChatHistoryService) AddChatMessage(ctx context.Context, appId int64, message string, messageType enum.ChatHistoryMessageTypeEnum, userId int64) error {
	return nil
}
func (nopChatHistoryService) DeleteByAppId(ctx context.Context, appId int64) error { return nil }
func (nopChatHistoryService) ListAppChatHistoryByPage(ctx context.Context, appId int64, pageSize int32, lastCreateTime time.Time, lastID int64, loginUser *api.UserVo) (*response.PageResponse[*model.ChatHistory], error) {
	return &response.PageResponse[*model.ChatHistory]{Records: []*model.ChatHistory{}}, nil
}
func (nopChatHistoryService) ListAllChatHistoryByPageForAdmin(ctx context.Context, pageNum int32, pageSize int32, queryRequest *api.NoCodeChatHistoryQueryRequest) (*response.PageResponse[*model.ChatHistory], error) {
	return &response.PageResponse[*model.ChatHistory]{Records: []*model.ChatHistory{}}, nil
}
func (nopChatHistoryService) LoadChatHistoryToMemory(ctx context.Context, appId int64, memoryStore store.MemoryStore, maxCount int) (int, error) {
	return 0, nil
}

func TestGenCodeStreamAndSave(t *testing.T) {
	if testing.Short() || os.Getenv("RUN_INTEGRATION") == "" {
		t.Skip("skipping integration test (set RUN_INTEGRATION=1 to enable)")
	}
	cfg := config.InitConfig()
	chatModel := llm.NewClaudeChatModel(cfg)
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	var chatHistoryService service.IChatHistoryService = nopChatHistoryService{}
	codeAgent := agent.NewCodeGenAgentFactory(chatModel, redisClient, chatHistoryService)

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
