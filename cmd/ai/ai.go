package main

import (
	"context"
	"fmt"
	"time"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/adk"
)

func main() {
	cfg := config.InitConfig().AI
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	model, err := claude.NewChatModel(ctx, &claude.Config{
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
		BaseURL: &cfg.BaseURL,
	})
	if err != nil {
		panic(fmt.Sprintf("model: %v", err))
	}
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
			msg, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				panic(err)
			}
			fmt.Println(msg.Content)
		}
	}

}
