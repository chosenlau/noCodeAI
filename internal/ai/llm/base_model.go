package llm

import (
	"context"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/cloudwego/eino-ext/components/model/openai"
)

type ChatModelWrapper struct {
	*openai.ChatModel
	ModelName string
}

func NewChatModel(cfg *config.Config) *ChatModelWrapper {
	model, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:  cfg.AI.APIKey,
		Model:   cfg.AI.Model,
		BaseURL: cfg.AI.BaseURL,
	})
	if err != nil {
		panic(err)
	}
	return &ChatModelWrapper{
		ChatModel: model,
		ModelName: cfg.AI.Model,
	}
}

func (c *ChatModelWrapper) GetModelName() string {
	return c.ModelName
}

func (c *ChatModelWrapper) GetChatModel() *openai.ChatModel {
	return c.ChatModel
}
