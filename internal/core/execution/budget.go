package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrBudgetExceeded = errors.New("execution budget exceeded")

type BudgetOptions struct {
	Timeout      time.Duration
	ModelCalls   int
	ToolCalls    int
	Retries      int
	Tokens       int64
	OutputTokens int
}

func DefaultBudgetOptions() BudgetOptions {
	return BudgetOptions{Timeout: 25 * time.Minute, ModelCalls: 100, ToolCalls: 200, Retries: 3, Tokens: 200000, OutputTokens: 8192}
}

type BudgetSnapshot struct {
	ModelCalls      int   `json:"modelCalls"`
	ToolCalls       int   `json:"toolCalls"`
	Retries         int   `json:"retries"`
	Tokens          int64 `json:"tokens"`
	EstimatedTokens int64 `json:"estimatedTokens"`
	ReservedTokens  int64 `json:"reservedTokens"`
}

// Budget belongs to one execution; all nodes and their concurrent calls share it.
type Budget struct {
	mu         sync.Mutex
	options    BudgetOptions
	used       BudgetSnapshot
	deadline   time.Time
	observer   func(BudgetSnapshot) error
	persistErr error
}

func NewBudget(opts BudgetOptions) *Budget {
	d := DefaultBudgetOptions()
	if opts.Timeout <= 0 {
		opts.Timeout = d.Timeout
	}
	if opts.ModelCalls <= 0 {
		opts.ModelCalls = d.ModelCalls
	}
	if opts.ToolCalls <= 0 {
		opts.ToolCalls = d.ToolCalls
	}
	if opts.Retries <= 0 {
		opts.Retries = d.Retries
	}
	if opts.Tokens <= 0 {
		opts.Tokens = d.Tokens
	}
	if opts.OutputTokens <= 0 {
		opts.OutputTokens = d.OutputTokens
	}
	return &Budget{options: opts, deadline: time.Now().Add(opts.Timeout)}
}

type budgetKey struct{}

func WithBudget(ctx context.Context, budget *Budget) context.Context {
	return context.WithValue(ctx, budgetKey{}, budget)
}
func GetBudget(ctx context.Context) *Budget { b, _ := ctx.Value(budgetKey{}).(*Budget); return b }
func (b *Budget) Deadline() time.Time       { return b.deadline }
func (b *Budget) Snapshot() BudgetSnapshot  { b.mu.Lock(); defer b.mu.Unlock(); return b.used }

// Restore only at a node safe point, with no in-flight model reservations.
func (b *Budget) Restore(used BudgetSnapshot, elapsed time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if used.ReservedTokens != 0 || elapsed < 0 || used.ModelCalls < 0 || used.ToolCalls < 0 || used.Retries < 0 || used.Tokens < 0 {
		return fmt.Errorf("invalid checkpoint budget")
	}
	b.used = used
	b.deadline = time.Now().Add(b.options.Timeout - elapsed)
	return b.checkLocked()
}
func (b *Budget) checkLocked() error {
	if b.persistErr != nil {
		return b.persistErr
	}
	if !time.Now().Before(b.deadline) {
		return fmt.Errorf("%w: execution time", ErrBudgetExceeded)
	}
	if b.used.Tokens >= b.options.Tokens {
		return fmt.Errorf("%w: tokens", ErrBudgetExceeded)
	}
	return nil
}
func (b *Budget) consume(counter *int, limit int, resource string) error {
	if err := b.checkLocked(); err != nil {
		return err
	}
	if *counter >= limit {
		return fmt.Errorf("%w: %s", ErrBudgetExceeded, resource)
	}
	*counter++
	return b.persistLocked()
}

// Observer runs synchronously so a failed durable budget write fails closed.
func (b *Budget) Observe(observer func(BudgetSnapshot) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.observer = observer
}
func (b *Budget) persistLocked() error {
	if b.observer != nil && b.persistErr == nil {
		b.persistErr = b.observer(b.used)
	}
	return b.persistErr
}
func (b *Budget) Validate() error { b.mu.Lock(); defer b.mu.Unlock(); return b.checkLocked() }
func (b *Budget) ConsumeModel() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consume(&b.used.ModelCalls, b.options.ModelCalls, "model calls")
}
func (b *Budget) ConsumeTool() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consume(&b.used.ToolCalls, b.options.ToolCalls, "tool calls")
}
func (b *Budget) ConsumeRetry() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consume(&b.used.Retries, b.options.Retries, "retries")
}

// ReserveModel bounds concurrent estimated token consumption. The estimate is
// deliberately byte-based; provider tokenization and billing remain authoritative.
// Finish must be called exactly once. Unknown usage charges the reservation.
func (b *Budget) ReserveModel(inputEstimate int64) (outputLimit int, finish func(int64), err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err = b.checkLocked(); err != nil {
		return
	}
	remaining := b.options.Tokens - b.used.Tokens - b.used.ReservedTokens - inputEstimate
	if remaining <= 0 {
		err = fmt.Errorf("%w: estimated input tokens", ErrBudgetExceeded)
		return
	}
	outputLimit = b.options.OutputTokens
	if int64(outputLimit) > remaining {
		outputLimit = int(remaining)
	}
	reserved := inputEstimate + int64(outputLimit)
	b.used.ReservedTokens += reserved
	if err = b.persistLocked(); err != nil {
		b.used.ReservedTokens -= reserved
		return 0, nil, err
	}
	var once sync.Once
	finish = func(actual int64) {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.used.ReservedTokens -= reserved
			if actual < 0 {
				actual = reserved
				b.used.EstimatedTokens += reserved
			}
			b.used.Tokens += actual
			_ = b.persistLocked()
		})
	}
	return
}

func (b *Budget) RecordTokens(tokens int64) {
	if tokens > 0 {
		b.mu.Lock()
		b.used.Tokens += tokens
		_ = b.persistLocked()
		b.mu.Unlock()
	}
}

// ModelSlots is process-wide, including legacy requests and summaries. A permit
// stays held until a streaming provider call is closed or reaches EOF.
var modelSlots = make(chan struct{}, 4)

func AcquireModel(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case modelSlots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-modelSlots
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(func() { <-modelSlots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
