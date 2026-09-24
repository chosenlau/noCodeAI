package llm

import (
	"context"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

type OpenAIChatModelWrapper struct {
	*openai.ChatModel
	ModelName string
}

func NewOpenAIChatModel(cfg *config.Config) *OpenAIChatModelWrapper {
	model, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey:  cfg.AI.APIKey,
		Model:   cfg.AI.Model,
		BaseURL: cfg.AI.BaseURL,
	})
	if err != nil {
		panic(err)
	}
	return &OpenAIChatModelWrapper{
		ChatModel: model,
		ModelName: cfg.AI.Model,
	}
}

func (c *OpenAIChatModelWrapper) GetModelName() string {
	return c.ModelName
}

func (c *OpenAIChatModelWrapper) GetChatModel() model.ToolCallingChatModel {
	return c.ChatModel
}
