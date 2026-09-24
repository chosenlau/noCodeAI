package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent/agentmiddleware"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ChatModelWrapperAdaptor interface {
	GetChatModel() model.ToolCallingChatModel
	GetModelName() string
}

type BaseAgent struct {
	model            model.ToolCallingChatModel
	modelName        string
	middleware       *agentmiddleware.AgentMiddleware
	metricsCollector *monitor.AiModelMetricsCollector
	checkpointStore  adk.CheckPointStore
}

func NewBaseAgent(model ChatModelWrapperAdaptor, metricsCollector *monitor.AiModelMetricsCollector, checkpointStore adk.CheckPointStore, agentMiddleware *agentmiddleware.AgentMiddleware) *BaseAgent {
	return &BaseAgent{
		model:            model.GetChatModel(),
		modelName:        model.GetModelName(),
		metricsCollector: metricsCollector,
		checkpointStore:  checkpointStore,
		middleware:       agentMiddleware,
	}
}

func (a *BaseAgent) NewAdkAgent(name, description, instruction string, tools []tool.BaseTool) *adk.ChatModelAgent {
	config := &adk.ChatModelAgentConfig{
		Name:        name,
		Description: description,
		Instruction: instruction,
		Model:       a.model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
				UnknownToolsHandler: func(ctx context.Context, name, input string) (string, error) {
					return fmt.Sprintf("閿欒: 娌℃湁杩欎釜鍚嶇О鐨勫伐鍏?%s", name), nil
				},
			},
		},
		MaxIterations: 50,
		ModelRetryConfig: &adk.ModelRetryConfig{
			MaxRetries: 3,
			IsRetryAble: func(ctx context.Context, err error) bool {
				if errors.Is(err, context.Canceled) {
					return false
				}
				return true
			},
		},
	}

	if a.middleware != nil {
		config.Handlers = []adk.ChatModelAgentMiddleware{a.middleware}
	}
	ctx := context.Background()
	agent, err := adk.NewChatModelAgent(ctx, config)
	if err != nil {
		logger.Errorf("鍒涘缓Agent澶辫触: %v", err)
		return nil
	}
	return agent
}

func (a *BaseAgent) Generate(ctx context.Context, messages []*schema.Message, adkAgent *adk.ChatModelAgent) (*schema.Message, error) {
	monitorContext := monitor.GetMonitorContext(ctx)

	if a.metricsCollector != nil && monitorContext != nil {
		defer a.metricsCollector.RecordResponseTimeStart(monitorContext.UserId, monitorContext.AppId, a.modelName)()
	}

	runnerConfig := adk.RunnerConfig{
		Agent:           adkAgent,
		EnableStreaming: false,
	}
	if a.checkpointStore != nil {
		runnerConfig.CheckPointStore = a.checkpointStore
	}

	runner := adk.NewRunner(ctx, runnerConfig)

	var iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]]
	if a.checkpointStore != nil {
		checkpointID, ok := ctx.Value("checkpointID").(string)
		if !ok || checkpointID == "" {
			err := errors.New("checkpointID not found in context")
			if a.metricsCollector != nil && monitorContext != nil {
				a.metricsCollector.RecordError(monitorContext.UserId, monitorContext.AppId, a.modelName, err.Error())
			}
			return nil, err
		}
		iter = runner.Run(ctx, messages, adk.WithCheckPointID(checkpointID))
	} else {
		iter = runner.Run(ctx, messages)
	}

	var resultMsg *schema.Message
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			if a.metricsCollector != nil && monitorContext != nil {
				a.metricsCollector.RecordError(monitorContext.UserId, monitorContext.AppId, a.modelName, event.Err.Error())
			}
			return nil, event.Err
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg, err := event.Output.MessageOutput.GetMessage()
			if err != nil {
				if a.metricsCollector != nil && monitorContext != nil {
					a.metricsCollector.RecordError(monitorContext.UserId, monitorContext.AppId, a.modelName, err.Error())
				}
				return nil, err
			}
			resultMsg = msg
		}
	}

	if a.metricsCollector != nil && monitorContext != nil {
		a.metricsCollector.RecordRequest(monitorContext.UserId, monitorContext.AppId, a.modelName, "success")
		if resultMsg != nil && resultMsg.ResponseMeta != nil && resultMsg.ResponseMeta.Usage != nil {
			tokenUsage := resultMsg.ResponseMeta.Usage
			a.metricsCollector.RecordTokenUsage(monitorContext.UserId, monitorContext.AppId, a.modelName,
				"prompt", float64(tokenUsage.PromptTokens))
			a.metricsCollector.RecordTokenUsage(monitorContext.UserId, monitorContext.AppId, a.modelName,
				"completion", float64(tokenUsage.CompletionTokens))
			a.metricsCollector.RecordTokenUsage(monitorContext.UserId, monitorContext.AppId, a.modelName,
				"total", float64(tokenUsage.PromptTokens+tokenUsage.CompletionTokens))
		}
	}
	if resultMsg != nil && resultMsg.ResponseMeta != nil {
		recordTokenUsage(ctx, resultMsg.ResponseMeta.Usage)
	}

	return resultMsg, nil
}

func (a *BaseAgent) GenerateStream(ctx context.Context, messages []*schema.Message, adkAgent *adk.ChatModelAgent) (*schema.StreamReader[*schema.Message], error) {
	monitorContext := monitor.GetMonitorContext(ctx)

	runnerConfig := adk.RunnerConfig{
		Agent:           adkAgent,
		EnableStreaming: true,
	}
	if a.checkpointStore != nil {
		runnerConfig.CheckPointStore = a.checkpointStore
	}

	runner := adk.NewRunner(ctx, runnerConfig)

	var iter *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]]
	if a.checkpointStore != nil {
		checkpointID, ok := ctx.Value("checkpointID").(string)
		if !ok || checkpointID == "" {
			err := errors.New("checkpointID not found in context")
			if a.metricsCollector != nil && monitorContext != nil {
				a.metricsCollector.RecordError(monitorContext.UserId, monitorContext.AppId, a.modelName, err.Error())
			}
			return nil, err
		}
		iter = runner.Run(ctx, messages, adk.WithCheckPointID(checkpointID))
	} else {
		iter = runner.Run(ctx, messages)
	}

	reader, writer := schema.Pipe[*schema.Message](2)

	go func() {
		defer writer.Close()
		var fullContent string
		var lastTokenUsage *schema.TokenUsage
		var streamErr error

		startTime := time.Now()

		for {
			if err := ctx.Err(); err != nil {
				streamErr = err
				_ = writer.Send(nil, err)
				return
			}
			event, ok := iter.Next()
			if !ok {
				break
			}

			if event.Err != nil {
				streamErr = event.Err
				if a.metricsCollector != nil && monitorContext != nil {
					a.metricsCollector.RecordError(monitorContext.UserId, monitorContext.AppId, a.modelName, event.Err.Error())
				}
				writer.Send(nil, event.Err)
				return
			}

			if event.Output != nil && event.Output.MessageOutput != nil {
				stream := event.Output.MessageOutput.MessageStream
				if stream != nil {
					for {
						if err := ctx.Err(); err != nil {
							streamErr = err
							_ = writer.Send(nil, err)
							return
						}
						msg, err := stream.Recv()
						if err == io.EOF {
							break
						}
						if err != nil {
							streamErr = err
							if a.metricsCollector != nil && monitorContext != nil {
								a.metricsCollector.RecordError(monitorContext.UserId, monitorContext.AppId, a.modelName, err.Error())
							}
							writer.Send(nil, err)
							return
						}
						if msg != nil {
							fullContent += msg.Content
							if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
								lastTokenUsage = msg.ResponseMeta.Usage
							}
							if writer.Send(msg, nil) {
								return
							}
						}
					}
				} else if event.Output.MessageOutput.Message != nil {
					msg := event.Output.MessageOutput.Message
					fullContent += msg.Content
					if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
						lastTokenUsage = msg.ResponseMeta.Usage
					}
					if writer.Send(msg, nil) {
						return
					}
				}
			}
		}

		if a.metricsCollector != nil && monitorContext != nil {
			duration := time.Since(startTime)
			a.metricsCollector.RecordResponseTime(monitorContext.UserId, monitorContext.AppId, a.modelName, duration)

			if streamErr == nil {
				a.metricsCollector.RecordRequest(monitorContext.UserId, monitorContext.AppId, a.modelName, "success")

				if lastTokenUsage != nil {
					a.metricsCollector.RecordTokenUsage(monitorContext.UserId, monitorContext.AppId, a.modelName,
						"input", float64(lastTokenUsage.PromptTokens))
					a.metricsCollector.RecordTokenUsage(monitorContext.UserId, monitorContext.AppId, a.modelName,
						"output", float64(lastTokenUsage.CompletionTokens))
					a.metricsCollector.RecordTokenUsage(monitorContext.UserId, monitorContext.AppId, a.modelName,
						"total", float64(lastTokenUsage.PromptTokens+lastTokenUsage.CompletionTokens))
				}
			}
		}
		recordTokenUsage(ctx, lastTokenUsage)
	}()

	return reader, nil
}

// 传token逻辑，开始执行时绑定
type TokenUsageRecorder interface {
	RecordTokenUsage(ctx context.Context, usage *schema.TokenUsage)
}

type tokenUsageRecorderContextKey struct{}

func WithTokenUsageRecorder(ctx context.Context, recorder TokenUsageRecorder) context.Context {
	return context.WithValue(ctx, tokenUsageRecorderContextKey{}, recorder)
}

func recordTokenUsage(ctx context.Context, usage *schema.TokenUsage) {
	if usage == nil {
		return
	}
	if recorder, ok := ctx.Value(tokenUsageRecorderContextKey{}).(TokenUsageRecorder); ok && recorder != nil {
		recorder.RecordTokenUsage(ctx, usage)
	}
}
