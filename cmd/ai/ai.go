package main

import (
	"context"
	"fmt"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
)

func main() {
	cfg := config.InitConfig().AI
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	model, _ := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		BaseURL: cfg.BaseURL,
	})
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model: model,
	})
	if err != nil {
		panic(err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agent})
	iter := runner.Query(ctx, "Hello, who are you?")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, _ := event.Output.MessageOutput.GetMessage()
			fmt.Println(msg.Content)
		}
	}

}
