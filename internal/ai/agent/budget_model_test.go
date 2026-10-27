package agent

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type budgetTestModel struct {
	calls       atomic.Int32
	outputLimit atomic.Int32
}

func (m *budgetTestModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *budgetTestModel) Generate(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.calls.Add(1)
	if limit := model.GetCommonOptions(nil, opts...).MaxTokens; limit != nil {
		m.outputLimit.Store(int32(*limit))
	}
	return &schema.Message{Content: "done", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5}}}, nil
}
func (m *budgetTestModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

type budgetTestRecorder struct{ tokens atomic.Int64 }

func (r *budgetTestRecorder) RecordTokenUsage(_ context.Context, u *schema.TokenUsage) {
	r.tokens.Add(int64(u.PromptTokens + u.CompletionTokens))
}

func TestBudgetProviderCountsEachAttemptAndRecordsOnce(t *testing.T) {
	provider := &budgetTestModel{}
	wrapped := &budgetModel{inner: provider}
	bound, err := wrapped.WithTools(nil)
	if err != nil {
		t.Fatal(err)
	}
	b := execution.NewBudget(execution.BudgetOptions{ModelCalls: 2, Tokens: 1000, OutputTokens: 50})
	r := &budgetTestRecorder{}
	ctx := WithTokenUsageRecorder(execution.WithBudget(context.Background(), b), r)
	if _, err = bound.Generate(ctx, []*schema.Message{{Content: "hello"}}); err != nil {
		t.Fatal(err)
	}
	stream, err := bound.Stream(ctx, []*schema.Message{{Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for {
		_, err = stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = bound.Generate(ctx, nil); !errors.Is(err, execution.ErrBudgetExceeded) {
		t.Fatalf("expected call budget error: %v", err)
	}
	if provider.calls.Load() != 2 || provider.outputLimit.Load() != 50 {
		t.Fatal("provider calls or output limit escaped budget")
	}
	if r.tokens.Load() != 30 || b.Snapshot().Tokens != 30 {
		t.Fatalf("usage should record once per provider call: recorder=%d budget=%+v", r.tokens.Load(), b.Snapshot())
	}
}

type blockedBudgetModel struct{ budgetTestModel }

func (m *blockedBudgetModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	r, w := schema.Pipe[*schema.Message](0)
	go func() { defer w.Close(); <-ctx.Done() }()
	return r, nil
}

func TestBudgetStreamCancellationReleasesProviderCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(execution.WithBudget(context.Background(), execution.NewBudget(execution.BudgetOptions{})))
	m := &budgetModel{inner: &blockedBudgetModel{}}
	stream, err := m.Stream(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan struct{})
	go func() { defer close(done); defer stream.Close(); stream.Recv() }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled stream did not terminate")
	}
	// All four global slots must become available after cancellation.
	waitCtx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	var releases []func()
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	for range 4 {
		release, err := execution.AcquireModel(waitCtx)
		if err != nil {
			t.Fatalf("leaked streaming slot: %v", err)
		}
		releases = append(releases, release)
	}
}
