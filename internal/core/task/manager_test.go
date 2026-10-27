package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandlerPanicBecomesFailure(t *testing.T) {
	_, err := callHandler(context.Background(), func(context.Context, Run, EmitFunc) (Result, error) {
		panic("tool failure")
	}, Run{}, nil)
	require.ErrorContains(t, err, "tool failure")
}

type replayStore struct {
	Store
	run           Run
	events        []Event
	missFirstRead bool
}

func (s *replayStore) GetRun(_ context.Context, id, user int64) (Run, error) {
	if id != s.run.ID || user != s.run.UserID {
		return Run{}, ErrNotFound
	}
	return s.run, nil
}
func (s *replayStore) ListEvents(_ context.Context, id, after int64) ([]Event, error) {
	if s.missFirstRead {
		s.missFirstRead = false
		return nil, nil
	}
	var result []Event
	for _, e := range s.events {
		if e.RunID == id && e.Sequence > after {
			result = append(result, e)
		}
	}
	return result, nil
}

func TestSubscriptionDrainsCompletionCommittedBetweenReads(t *testing.T) {
	s := &replayStore{
		run:           Run{ID: 1, UserID: 2, Status: StatusCompleted, LastSequence: 1},
		events:        []Event{{RunID: 1, Sequence: 1, Type: EventDone}},
		missFirstRead: true,
	}
	m := NewManager(s, Options{Enabled: true})
	sub, err := m.Subscribe(context.Background(), 1, 2, 0)
	require.NoError(t, err)
	defer sub.Close()
	e, err := sub.Next(context.Background())
	require.NoError(t, err)
	require.Equal(t, EventDone, e.Type)
}
func (s *replayStore) GetSourceFiles(context.Context, int64) (map[string]string, error) {
	return map[string]string{"index.html": "saved"}, nil
}

func TestSubscriptionReplaysSnapshotAndEnforcesOwnership(t *testing.T) {
	s := &replayStore{run: Run{ID: 1, UserID: 2, Status: StatusCompleted}, events: []Event{
		{RunID: 1, Sequence: 1, Type: EventRunCreated},
		{RunID: 1, Sequence: 2, Type: EventCodeCompleted, Payload: json.RawMessage(`{"sourceVersionId":"7"}`)},
		{RunID: 1, Sequence: 3, Type: EventDone},
	}}
	m := NewManager(s, Options{Enabled: true})
	_, err := m.Subscribe(context.Background(), 1, 3, 0)
	require.ErrorIs(t, err, ErrNotFound)
	sub, err := m.Subscribe(context.Background(), 1, 2, 1)
	require.NoError(t, err)
	defer sub.Close()
	e, err := sub.Next(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(2), e.Sequence)
	require.JSONEq(t, `{"index.html":"saved"}`, string(e.Payload))
	e, err = sub.Next(context.Background())
	require.NoError(t, err)
	require.Equal(t, EventDone, e.Type)
	_, err = sub.Next(context.Background())
	require.ErrorIs(t, err, io.EOF)
}

func TestRuntimeRejectsDisabledStoppedAndInvalidRequests(t *testing.T) {
	m := NewManager(nil, Options{})
	_, _, err := m.Start(context.Background(), StartRequest{})
	require.ErrorIs(t, err, ErrDisabled)
	m = NewManager(nil, Options{Enabled: true})
	_, _, err = m.Start(context.Background(), StartRequest{})
	require.Error(t, err)
	require.NoError(t, m.Shutdown(context.Background()))
	_, _, err = m.Start(context.Background(), StartRequest{})
	require.True(t, errors.Is(err, ErrStopped))
}

type lifecycleStore struct {
	Store
	complete func(Result) error
	fail     func(string)
}

type suspensionStore struct {
	*lifecycleStore
	ControlStore
	suspend func(string) error
}

func (s *suspensionStore) GetRunInternal(context.Context, int64) (Run, error) {
	return Run{ID: 1, Status: StatusRunning}, nil
}

func (s *suspensionStore) Suspend(_ context.Context, _ int64, status string) error {
	return s.suspend(status)
}

func TestManagerReleasesExecutionBeforeMakingSuspensionResumable(t *testing.T) {
	for _, atPublication := range []bool{false, true} {
		t.Run(fmt.Sprintf("atPublication=%v", atPublication), func(t *testing.T) {
			releases, suspensions := 0, 0
			s := &suspensionStore{lifecycleStore: &lifecycleStore{}}
			s.complete = func(Result) error { return &Suspended{Status: StatusPaused} }
			s.fail = func(message string) { t.Fatalf("suspension became failure: %s", message) }
			s.suspend = func(status string) error {
				require.Equal(t, 1, releases, "a resumed task must not race the old execution lease")
				require.Equal(t, StatusPaused, status)
				suspensions++
				return nil
			}
			m := NewManager(s, Options{Enabled: true})
			m.wg.Add(1)
			m.sem <- struct{}{}
			m.execute(Run{ID: 1}, func(context.Context, Run, EmitFunc) (Result, error) {
				result := Result{Release: func() { releases++ }}
				if atPublication {
					return result, nil
				}
				return result, &Suspended{Status: StatusPaused}
			})
			require.Equal(t, 1, releases)
			require.Equal(t, 1, suspensions)
		})
	}
}

func (s *lifecycleStore) Complete(_ context.Context, _ int64, result Result) (Run, error) {
	return Run{}, s.complete(result)
}
func (s *lifecycleStore) Fail(_ context.Context, _ int64, message string, _ bool) (Run, error) {
	s.fail(message)
	return Run{}, nil
}

func TestManagerKeepsLeaseThroughPublicationAndRejectsCanceledExecution(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%v", canceled), func(t *testing.T) {
			released, completed, afterCommit, failed := false, false, false, false
			s := &lifecycleStore{}
			s.complete = func(result Result) error { require.False(t, released); completed = true; return nil }
			s.fail = func(message string) {
				require.False(t, released)
				require.Contains(t, message, "canceled")
				failed = true
			}
			m := NewManager(s, Options{Enabled: true})
			m.wg.Add(1)
			m.sem <- struct{}{}
			m.execute(Run{ID: 1}, func(ctx context.Context, _ Run, _ EmitFunc) (Result, error) {
				execCtx, cancel := context.WithCancel(ctx)
				if canceled {
					cancel()
				}
				// A healthy execution's cancellation belongs to Release, after publish.
				validate := execCtx.Err
				return Result{Validate: validate, Release: func() { released = true; cancel() }, AfterCommit: func() { require.True(t, completed); require.False(t, released); afterCommit = true }}, nil
			})
			require.True(t, released)
			require.Equal(t, !canceled, completed)
			require.Equal(t, canceled, failed)
			require.Equal(t, !canceled, afterCommit)
		})
	}
}
