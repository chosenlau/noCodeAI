package contextmgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func testOptions() Options { return Options{Window: 12000, Output: 1000, Margin: 1000} }

func toolGroup(id string, size int) []*schema.Message {
	return []*schema.Message{
		{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: id, Function: schema.FunctionCall{Name: "readFile", Arguments: `{"path":"index.html"}`}}}},
		{Role: schema.Tool, ToolCallID: id, Content: strings.Repeat("x", size)},
	}
}

func TestReductionPreservesLatestGroupAndOriginalMessages(t *testing.T) {
	msgs := []*schema.Message{schema.SystemMessage("instruction"), schema.UserMessage("build")}
	msgs = append(msgs, toolGroup("old", 7400)...)
	msgs = append(msgs, toolGroup("latest", 900)...)
	// A parallel tool result belongs to the same latest assistant call group.
	msgs[4].ToolCalls = append(msgs[4].ToolCalls, schema.ToolCall{ID: "parallel", Function: schema.FunctionCall{Name: "readFile", Arguments: "{}"}})
	msgs = append(msgs, &schema.Message{Role: schema.Tool, ToolCallID: "parallel", Content: strings.Repeat("y", 900)})
	out, err := New(testOptions(), "code", nil, nil).Assemble(context.Background(), msgs, 0)
	require.NoError(t, err)
	require.Contains(t, out[3].Content, "sha256=")
	require.Len(t, msgs[3].Content, 7400, "original tool history must remain untouched")
	require.Same(t, msgs[4], out[4])
	require.Same(t, msgs[5], out[5])
	require.Same(t, msgs[6], out[6])
}

func TestSummaryCommitsBeforeReplacingAndPinsCurrentState(t *testing.T) {
	msgs := []*schema.Message{schema.SystemMessage("system"), schema.UserMessage(strings.Repeat("old", 2100)), schema.AssistantMessage("finished", nil), schema.UserMessage("current request")}
	var saved Snapshot
	manager := New(testOptions(), "code", func(ctx context.Context, r Request) (Summary, error) {
		require.True(t, Disabled(ctx), "summary cannot recursively compact")
		require.Len(t, r.Materials, 2)
		return Summary{CurrentGoal: "current goal", NextSafeAction: "read file", ImportantFindings: []Finding{{Text: "old work", Sources: []string{r.Materials[0].ID}}}}, nil
	}, &Binding{
		Protected: func() string { return `{"todos":["build","check"]}` },
		Commit: func(_ context.Context, name string, s Snapshot) error {
			require.Equal(t, "code", name)
			saved = s
			return nil
		},
	})
	out, err := manager.Assemble(context.Background(), msgs, 800)
	require.NoError(t, err)
	require.Equal(t, 1, saved.Version)
	require.Contains(t, out[0].Content, "current goal")
	joined := ""
	for _, msg := range out {
		joined += msg.Content
	}
	require.Contains(t, joined, "current request")
	require.Contains(t, joined, "system")
	require.Contains(t, joined, "todos")
	require.NotContains(t, joined, strings.Repeat("old", 100))
	// Re-entry into this agent restores the committed working memory once.
	restored := New(testOptions(), "code", nil, &Binding{Load: func(string) Snapshot { return saved }})
	out, err = restored.Assemble(context.Background(), []*schema.Message{schema.UserMessage("continue")}, 0)
	require.NoError(t, err)
	out, err = restored.Assemble(context.Background(), out, 0)
	require.NoError(t, err)
	require.Len(t, out, 2)
}

func TestFailedSummaryFallsBackOrBlocksWithoutReadyEvent(t *testing.T) {
	for _, size := range []int{7300, 8700} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var events []string
			blocked := false
			manager := New(testOptions(), "code", func(context.Context, Request) (Summary, error) {
				return Summary{CurrentGoal: "goal", NextSafeAction: "continue", ImportantFindings: []Finding{{Text: "invented", Sources: []string{"unknown"}}}}, nil
			}, &Binding{Notify: func(s string) { events = append(events, s) }, Blocked: func(context.Context) error { blocked = true; return nil }})
			msgs := []*schema.Message{schema.UserMessage(strings.Repeat("a", size)), schema.AssistantMessage("old", nil), schema.UserMessage("current")}
			out, err := manager.Assemble(context.Background(), msgs, 0)
			if size < 8500 {
				require.NoError(t, err)
				require.Len(t, out, 3)
				require.False(t, blocked)
				require.Equal(t, []string{"context_compacting", "context_ready"}, events)
			} else {
				require.ErrorIs(t, err, ErrTooLarge)
				require.True(t, blocked)
				require.Equal(t, []string{"context_compacting"}, events)
			}
		})
	}
}

func TestCommitFailureRetainsPreviousSnapshot(t *testing.T) {
	persistenceErr := errors.New("checkpoint failed")
	previous := Snapshot{Version: 3, Summary: Summary{CurrentGoal: "old", NextSafeAction: "read", UserConstraints: []string{"keep layout"}}}
	manager := New(testOptions(), "code", func(context.Context, Request) (Summary, error) {
		return Summary{CurrentGoal: "new", NextSafeAction: "write", UserConstraints: []string{"keep layout"}}, nil
	}, &Binding{Load: func(string) Snapshot { return previous }, Commit: func(context.Context, string, Snapshot) error { return persistenceErr }})
	_, err := manager.Assemble(context.Background(), []*schema.Message{schema.UserMessage(strings.Repeat("a", 7300)), schema.UserMessage("current")}, 0)
	require.ErrorIs(t, err, persistenceErr)
	require.Equal(t, previous, manager.snapshot)
}

func TestCannotSilentlyLosePreviousConstraint(t *testing.T) {
	err := validate(Summary{CurrentGoal: "goal", NextSafeAction: "continue"}, Request{Previous: Summary{UserConstraints: []string{"no architecture change"}}})
	require.Error(t, err)
}
