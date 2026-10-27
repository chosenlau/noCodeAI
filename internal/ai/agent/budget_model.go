package agent

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// budgetModel wraps the provider, below ADK's retry middleware. Every attempt
// consumes capacity, including attempts retried inside the ADK agent.
type budgetModel struct {
	inner        model.ToolCallingChatModel
	toolEstimate int64
	outputLimit  int
}

func (m *budgetModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(tools)
	if err != nil {
		return nil, err
	}
	return &budgetModel{inner: bound, toolEstimate: int64(len(encoded)), outputLimit: m.outputLimit}, nil
}

func (m *budgetModel) limitOutput(opts []model.Option) []model.Option {
	if m.outputLimit <= 0 {
		return opts
	}
	common := model.GetCommonOptions(nil, opts...)
	if common.MaxTokens != nil && *common.MaxTokens > 0 && *common.MaxTokens < m.outputLimit {
		return opts
	}
	return append(append([]model.Option{}, opts...), model.WithMaxTokens(m.outputLimit))
}

func beginModel(ctx context.Context, msgs []*schema.Message, opts []model.Option, toolEstimate int64) (context.Context, []model.Option, func(*schema.TokenUsage), error) {
	deadline := time.Now().Add(25 * time.Minute)
	budget := execution.GetBudget(ctx)
	if budget != nil {
		deadline = budget.Deadline()
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	release, err := execution.AcquireModel(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	finishTokens := func(int64) {}
	cleanup := func() { release(); cancel() }
	if budget != nil {
		if err = budget.ConsumeModel(); err != nil {
			cleanup()
			return nil, nil, nil, err
		}
		encodedMessages, encodeErr := json.Marshal(msgs)
		if encodeErr != nil {
			cleanup()
			return nil, nil, nil, encodeErr
		}
		estimate := toolEstimate + int64(len(encodedMessages))
		common := model.GetCommonOptions(nil, opts...)
		if len(common.Tools) > 0 {
			encoded, encodeErr := json.Marshal(common.Tools)
			if encodeErr != nil {
				cleanup()
				return nil, nil, nil, encodeErr
			}
			estimate += int64(len(encoded))
		}
		var limit int
		limit, finishTokens, err = budget.ReserveModel(estimate)
		if err != nil {
			cleanup()
			return nil, nil, nil, err
		}
		if common.MaxTokens != nil && *common.MaxTokens > 0 && *common.MaxTokens < limit {
			limit = *common.MaxTokens
		}
		opts = append(append([]model.Option{}, opts...), model.WithMaxTokens(limit))
	}
	var once sync.Once
	finish := func(usage *schema.TokenUsage) {
		once.Do(func() {
			actual := int64(-1)
			if usage != nil {
				actual = int64(usage.PromptTokens + usage.CompletionTokens)
			}
			finishTokens(actual)
			recordTokenUsage(ctx, usage)
			cleanup()
		})
	}
	return ctx, opts, finish, nil
}

func (m *budgetModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	ctx, opts, finish, err := beginModel(ctx, msgs, m.limitOutput(opts), m.toolEstimate)
	if err != nil {
		return nil, err
	}
	var usage *schema.TokenUsage
	defer func() { finish(usage) }()
	msg, err := m.inner.Generate(ctx, msgs, opts...)
	if msg != nil && msg.ResponseMeta != nil {
		usage = msg.ResponseMeta.Usage
	}
	return msg, err
}

func (m *budgetModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	ctx, opts, finish, err := beginModel(ctx, msgs, m.limitOutput(opts), m.toolEstimate)
	if err != nil {
		return nil, err
	}
	stream, err := m.inner.Stream(ctx, msgs, opts...)
	if err != nil {
		finish(nil)
		return nil, err
	}
	reader, writer := schema.Pipe[*schema.Message](0)
	reader.SetAutomaticClose()
	stream.SetAutomaticClose()
	go func() {
		var usage *schema.TokenUsage
		stop := context.AfterFunc(ctx, func() { stream.Close(); reader.Close() })
		defer func() { stop(); stream.Close(); finish(usage); writer.Close() }()
		for {
			msg, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					writer.Send(nil, err)
				}
				return
			}
			if msg != nil && msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil {
				usage = msg.ResponseMeta.Usage
			}
			if writer.Send(msg, nil) {
				return
			}
		}
	}()
	return reader, nil
}
