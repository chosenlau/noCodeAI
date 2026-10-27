package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

type Manager struct {
	store      Store
	options    Options
	hub        *eventHub
	handlers   map[string]Handler
	executions map[int64]*executionHandle

	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wake     chan struct{}
	sem      chan struct{}
	wg       sync.WaitGroup
	startOne sync.Once
	stopOne  sync.Once
	release  func()
	startErr error
	stopped  chan struct{}
}

func NewManager(dbStore Store, options Options) *Manager {
	if options.WorkerCount <= 0 {
		options.WorkerCount = 2
	}
	if options.PollInterval <= 0 {
		options.PollInterval = 500 * time.Millisecond
	}
	if options.ShutdownTimeout <= 0 {
		options.ShutdownTimeout = 20 * time.Second
	}
	if options.OutboxBatchSize <= 0 {
		options.OutboxBatchSize = 100
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		store:      dbStore,
		options:    options,
		hub:        newEventHub(),
		handlers:   make(map[string]Handler),
		executions: make(map[int64]*executionHandle),
		ctx:        ctx,
		cancel:     cancel,
		wake:       make(chan struct{}, 1),
		sem:        make(chan struct{}, options.WorkerCount),
		stopped:    make(chan struct{}),
	}
}

func (m *Manager) Enabled() bool { return m != nil && m.options.Enabled }

func (m *Manager) Register(kind string, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[kind] = handler
}

func (m *Manager) StartWorkers() error {
	if !m.Enabled() {
		return nil
	}
	m.startOne.Do(func() {
		m.release, m.startErr = m.store.AcquireRuntime(m.ctx)
		if m.startErr != nil {
			return
		}
		startupCtx, startupCancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer startupCancel()
		if err := m.store.InterruptStale(startupCtx); err != nil {
			m.release()
			m.release = nil
			m.startErr = fmt.Errorf("mark stale task runs interrupted: %w", err)
			return
		}
		m.wg.Add(1)
		go m.outboxLoop()
	})
	return m.startErr
}

func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.stopOne.Do(func() {
		m.cancel()
		go func() {
			m.wg.Wait()
			if m.release != nil {
				m.release()
			}
			close(m.stopped)
		}()
	})
	select {
	case <-m.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Start(ctx context.Context, request StartRequest) (Run, bool, error) {
	if !m.Enabled() {
		return Run{}, false, ErrDisabled
	}
	if m.ctx.Err() != nil {
		return Run{}, false, ErrStopped
	}
	if request.AppID <= 0 || request.UserID <= 0 || request.Kind == "" || len(request.Kind) > 32 || request.RequestID == "" || len(request.RequestID) > 128 || !json.Valid(request.Input) {
		return Run{}, false, fmt.Errorf("invalid task request")
	}
	run, created, err := m.store.CreateRun(ctx, request)
	if err != nil {
		return run, false, err
	}
	if created {
		m.notifyWorker()
		m.hub.notify(run.ID)
	}
	return run, created, nil
}

func (m *Manager) Get(ctx context.Context, runID, userID int64) (Run, error) {
	if !m.Enabled() {
		return Run{}, ErrDisabled
	}
	return m.store.GetRun(ctx, runID, userID)
}

func (m *Manager) Active(ctx context.Context, appID, userID int64) (Run, error) {
	if !m.Enabled() {
		return Run{}, ErrDisabled
	}
	return m.store.FindActiveRun(ctx, appID, userID)
}

func (m *Manager) Subscribe(ctx context.Context, runID, userID, afterSequence int64) (*Subscription, error) {
	if _, err := m.Get(ctx, runID, userID); err != nil {
		return nil, err
	}
	notify, unsubscribe := m.hub.subscribe(runID)
	return &Subscription{
		manager:     m,
		runID:       runID,
		userID:      userID,
		after:       afterSequence,
		notify:      notify,
		unsubscribe: unsubscribe,
	}, nil
}

func (m *Manager) outboxLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		case <-m.wake:
		}
		if err := m.store.CheckRuntime(m.ctx); err != nil {
			log.Printf("task runtime ownership lost: %v", err)
			m.cancel()
			return
		}
		m.drainOutbox()
	}
}

func (m *Manager) drainOutbox() {
	for {
		messages, err := m.store.ClaimOutbox(m.ctx, m.options.OutboxBatchSize)
		if err != nil || len(messages) == 0 {
			return
		}
		for _, message := range messages {
			if err := m.dispatch(message); err != nil {
				_ = m.store.RetryOutbox(context.Background(), message.ID, message.Attempts+1, err.Error())
				continue
			}
			if err := m.store.MarkOutboxDelivered(m.ctx, message.ID); err != nil {
				log.Printf("task outbox delivery acknowledgement failed: %v", err)
			}
		}
	}
}

func (m *Manager) dispatch(message OutboxMessage) error {
	switch message.EventType {
	case OutboxEventAvailable:
		m.hub.notify(message.AggregateID)
		return nil
	case OutboxTaskReady:
		run, err := m.store.GetRunInternal(m.ctx, message.AggregateID)
		if err != nil {
			return err
		}
		if run.Status != StatusPending {
			return nil
		}
		m.mu.RLock()
		handler := m.handlers[run.Kind]
		m.mu.RUnlock()
		if handler == nil {
			return fmt.Errorf("no task handler registered for kind %q", run.Kind)
		}
		select {
		case m.sem <- struct{}{}:
		case <-m.ctx.Done():
			return m.ctx.Err()
		default:
			return fmt.Errorf("task worker capacity exhausted")
		}
		running, err := m.store.MarkRunning(m.ctx, run.ID)
		if err != nil {
			<-m.sem
			return err
		}
		m.wg.Add(1)
		go m.execute(running, handler)
		return nil
	default:
		return fmt.Errorf("unsupported outbox event type %q", message.EventType)
	}
}

func (m *Manager) execute(run Run, handler Handler) {
	defer m.wg.Done()
	defer func() { <-m.sem; m.notifyWorker() }()
	// A bounded Runtime context owns execution, independently of SSE observers.
	runCtx, cancel := context.WithTimeout(m.ctx, 25*time.Minute)
	defer cancel()
	handle := &executionHandle{cancel: cancel}
	m.mu.Lock()
	m.executions[run.ID] = handle
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if m.executions[run.ID] == handle {
			delete(m.executions, run.ID)
		}
		m.mu.Unlock()
	}()
	// Covers cancellation between dispatch and registration of the handle.
	if _, ok := m.store.(ControlStore); ok {
		if current, err := m.store.GetRunInternal(runCtx, run.ID); err != nil || current.Status == StatusCancelling {
			cancel()
		}
	}
	m.hub.notify(run.ID)

	emit := func(eventType string, payload any) error {
		event, emitErr := m.store.AppendEvent(runCtx, run.ID, eventType, payload)
		if emitErr == nil {
			m.hub.notify(event.RunID)
			m.notifyWorker()
		}
		return emitErr
	}
	result, executionErr := callHandler(runCtx, handler, run, emit)
	if result.Release != nil {
		release := result.Release
		var once sync.Once
		result.Release = func() { once.Do(release) }
		defer result.Release()
	}
	if executionErr == nil {
		executionErr = runCtx.Err()
	}
	if executionErr == nil && result.Validate != nil {
		executionErr = result.Validate()
	}
	if executionErr != nil {
		var suspended *Suspended
		if errors.As(executionErr, &suspended) {
			if store, ok := m.store.(ControlStore); ok {
				if result.Release != nil {
					result.Release()
				}
				finalCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
				err := store.Suspend(finalCtx, run.ID, suspended.Status)
				done()
				if err != nil {
					m.failRun(run.ID, err.Error(), false)
				}
				m.hub.notify(run.ID)
				m.notifyWorker()
				return
			}
		}
		interrupted := m.ctx.Err() != nil
		m.failRun(run.ID, executionErr.Error(), interrupted)
		m.hub.notify(run.ID)
		m.notifyWorker()
		return
	}
	finalCtx, finalCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finalCancel()
	if _, err := m.store.Complete(finalCtx, run.ID, result); err != nil {
		var suspended *Suspended
		if errors.As(err, &suspended) {
			if result.Release != nil {
				result.Release()
			}
			if store, ok := m.store.(ControlStore); ok {
				if stopErr := store.Suspend(finalCtx, run.ID, suspended.Status); stopErr != nil {
					m.failRun(run.ID, stopErr.Error(), false)
				}
			} else {
				m.failRun(run.ID, err.Error(), false)
			}
		} else {
			m.failRun(run.ID, err.Error(), false)
		}
	} else if result.AfterCommit != nil {
		// Publication has committed. Metadata is best effort and cannot roll it back.
		callAfterCommit(run.ID, result.AfterCommit)
	}
	m.hub.notify(run.ID)
	m.notifyWorker()
}

func callAfterCommit(runID int64, afterCommit func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("task %d post-publication metadata panicked: %v", runID, recovered)
		}
	}()
	afterCommit()
}

func (m *Manager) failRun(runID int64, message string, interrupted bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.store.Fail(ctx, runID, message, interrupted); err != nil {
		log.Printf("task %d terminal status persistence failed: %v", runID, err)
	}
}

func callHandler(ctx context.Context, handler Handler, run Run, emit EmitFunc) (result Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("task handler panicked: %v", recovered)
		}
	}()
	return handler(ctx, run, emit)
}

func (m *Manager) notifyWorker() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

type Subscription struct {
	manager     *Manager
	runID       int64
	userID      int64
	after       int64
	notify      <-chan struct{}
	unsubscribe func()
	closeOnce   sync.Once
}

func (s *Subscription) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(s.unsubscribe)
}

func (s *Subscription) Next(ctx context.Context) (Event, error) {
	for {
		events, err := s.manager.store.ListEvents(ctx, s.runID, s.after)
		if err != nil {
			return Event{}, err
		}
		if len(events) > 0 {
			event := events[0]
			if event.Type == EventCodeCompleted {
				var ref struct {
					SourceVersionID int64 `json:"sourceVersionId,string"`
				}
				if json.Unmarshal(event.Payload, &ref) == nil && ref.SourceVersionID > 0 {
					files, filesErr := s.manager.store.GetSourceFiles(ctx, ref.SourceVersionID)
					if filesErr != nil {
						return Event{}, filesErr
					}
					event.Payload, _ = json.Marshal(files)
				}
			}
			s.after = event.Sequence
			return event, nil
		}
		run, err := s.manager.store.GetRun(ctx, s.runID, s.userID)
		if err != nil {
			return Event{}, err
		}
		if run.Terminal() || run.Status == StatusPaused || run.Status == StatusAwaitingApproval || run.Status == StatusRecoverable {
			// Completion can commit between ListEvents and GetRun. Drain the
			// final committed events before closing this subscription.
			if run.LastSequence > s.after {
				continue
			}
			return Event{}, io.EOF
		}
		timer := time.NewTimer(15 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Event{}, ctx.Err()
		case <-s.notify:
			timer.Stop()
		case <-timer.C:
			return Event{RunID: s.runID, Type: "heartbeat", Payload: json.RawMessage(`"ping"`)}, nil
		}
	}
}
