package agent

import (
	"context"
	"errors"
	"io"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ChatModelWrapperAdaptor interface {
	GetChatModel() model.ToolCallingChatModel
	GetModelName() string
}

type BaseAgent struct {
	model       model.ToolCallingChatModel
	modelName   string
	memoryStore *store.RedisMemoryStore
}

func NewBaseAgent(model ChatModelWrapperAdaptor, memoryStore *store.RedisMemoryStore) *BaseAgent {
	return &BaseAgent{
		model:       model.GetChatModel(),
		modelName:   model.GetModelName(),
		memoryStore: memoryStore,
	}
}

func (a *BaseAgent) NewAdkAgent(name, description, instruction string, tools []*tool.BaseTool) *adk.ChatModelAgent {
	ctx := context.Background()

	config := &adk.ChatModelAgentConfig{
		Name:          name,
		Description:   description,
		Instruction:   instruction,
		Model:         a.model,
		MaxIterations: 50,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries: 5,
			IsRetryAble: func(ctx context.Context, err error) bool {
				if errors.Is(err, context.Canceled) {
					return false
				}
				return true
			},
		},
	}

	agent, err := adk.NewChatModelAgent(ctx, config)
	if err != nil {
		logger.Errorf("Agent creation failed: %v", err)
		return nil
	}
	return agent
}

func (a *BaseAgent) Generate(ctx context.Context, userMessage string, chatTemplate prompt.ChatTemplate, adkAgent *adk.ChatModelAgent) (*schema.Message, error) {
	format, err := chatTemplate.Format(ctx, map[string]any{
		"content": userMessage,
	})
	if err != nil {
		return nil, err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           adkAgent,
		EnableStreaming: false,
	})

	iter := runner.Run(ctx, format)

	var resultMsg *schema.Message
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return nil, event.Err
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				return nil, err
			}
			resultMsg = msg
		}
	}

	return resultMsg, nil
}

func (a *BaseAgent) GenerateStream(ctx context.Context, userMessage string, chatTemplate prompt.ChatTemplate, adkAgent *adk.ChatModelAgent) (*schema.StreamReader[*schema.Message], error) {
	historyMessage, err := a.memoryStore.GetMessages(ctx)
	if err != nil {
		return nil, err
	}
	format, err := chatTemplate.Format(ctx, map[string]any{
		"content": userMessage,
		"history": historyMessage,
	})
	if err != nil {
		return nil, err
	}
	err = a.memoryStore.AppendMessage(ctx, schema.UserMessage(userMessage))
	if err != nil {
		return nil, err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           adkAgent,
		EnableStreaming: true,
	})

	iter := runner.Run(ctx, format)

	reader, writer := schema.Pipe[*schema.Message](2)

	go func() {
		defer writer.Close()
		var fullContent string
		for {
			event, ok := iter.Next()
			if !ok {
				break
			}
			if event.Err != nil {
				writer.Send(nil, event.Err)
				return
			}

			if event.Output != nil && event.Output.MessageOutput != nil {
				stream := event.Output.MessageOutput.MessageStream
				if stream != nil {
					for {
						msg, err := stream.Recv()
						if err == io.EOF {
							break
						}
						if err != nil {
							writer.Send(nil, err)
							return
						}
						if msg != nil {
							fullContent += msg.Content
							writer.Send(msg, nil)
						}
					}
				}
			}
		}
		err := a.memoryStore.AppendMessage(ctx, schema.AssistantMessage(fullContent, nil))
		if err != nil {
			logger.Errorf("保存对话记忆失败: %v", err)
		}
	}()

	return reader, nil
}
