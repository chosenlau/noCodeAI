package logic

import (
	"errors"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestGraphRunPersistsSemanticProgressWithoutRawTokens(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](8)
	for _, content := range []string{
		"raw model token",
		`{"type":"step_started","currentStep":"代码生成"}`,
		`{"type":"tool_request","id":"call-1","name":"write_file"}`,
		`{"type":"tool_executed","id":"call-1","name":"write_file"}`,
		`{"type":"step_completed","currentStep":"代码生成"}`,
		`{"type":"heartbeat"}`,
	} {
		writer.Send(&schema.Message{Content: content}, nil)
	}
	writer.Close()
	defer reader.Close()
	var types []string
	err := consumeGraphRunStream(reader, func(kind string, payload any) error {
		types = append(types, kind)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{task.EventStepStarted, task.EventToolRequest, task.EventToolExecuted, task.EventStepCompleted}, types)
}

func TestGraphRunStopsWhenProgressCannotBePersisted(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Send(&schema.Message{Content: `{"type":"step_started"}`}, nil)
	writer.Close()
	defer reader.Close()
	want := errors.New("event transaction unavailable")
	err := consumeGraphRunStream(reader, func(string, any) error { return want })
	require.ErrorIs(t, err, want)
}
