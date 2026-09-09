package llm

import (
	"context"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
)

type ClaudeChatModelWrapper struct {
	*claude.ChatModel
	ModelName string
}

func NewClaudeChatModel(cfg *config.Config) *ClaudeChatModelWrapper {
	model, err := claude.NewChatModel(context.Background(), &claude.Config{
		APIKey:  cfg.AI.APIKey,
		Model:   cfg.AI.Model,
		BaseURL: &cfg.AI.BaseURL,
	})
	if err != nil {
		panic(err)
	}
	return &ClaudeChatModelWrapper{
		ChatModel: model,
		ModelName: cfg.AI.Model,
	}
}

func (c *ClaudeChatModelWrapper) GetModelName() string {
	return c.ModelName
}

func (c *ClaudeChatModelWrapper) GetChatModel() model.ToolCallingChatModel {
	return c.ChatModel
}
