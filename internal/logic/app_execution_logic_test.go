package logic

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestExecutionStreamPublishesBeforeReleasing(t *testing.T) {
	released := make(chan struct{})
	published := false
	source := schema.StreamReaderFromArray([]*schema.Message{{Content: "draft"}})
	stream := finishExecutionStream(context.Background(), source, func() error {
		published = true
		return nil
	}, func() {
		if !published {
			panic("lease released before publication")
		}
		close(released)
	})
	defer stream.Close()
	msg, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "draft", msg.Content)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("execution was not released")
	}
}

func TestExecutionStreamPublicationErrorIsReturned(t *testing.T) {
	failure := errors.New("fenced publication")
	stream := finishExecutionStream(context.Background(), schema.StreamReaderFromArray([]*schema.Message{}), func() error {
		return failure
	}, func() {})
	defer stream.Close()
	_, err := stream.Recv()
	require.ErrorIs(t, err, failure)
}

func TestCanceledExecutionUnblocksBackpressureWithoutPublishing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	released := make(chan struct{})
	published := make(chan struct{}, 1)
	items := make([]*schema.Message, 20)
	for i := range items {
		items[i] = &schema.Message{Content: "draft"}
	}
	stream := finishExecutionStream(ctx, schema.StreamReaderFromArray(items), func() error {
		published <- struct{}{}
		return nil
	}, func() { close(released) })
	defer stream.Close()
	cancel()
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("cancellation left the execution blocked on its observer")
	}
	require.Empty(t, published)
}
