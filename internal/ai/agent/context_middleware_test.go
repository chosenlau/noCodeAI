package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type compactLoopModel struct {
	turn      int
	summaries int
	inputs    [][]*schema.Message
}

func (m *compactLoopModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *compactLoopModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	if len(messages) > 0 && messages[0].Content == workingSummaryInstruction {
		m.summaries++
		var req contextmgr.Request
		if err := json.Unmarshal([]byte(messages[1].Content), &req); err != nil {
			return nil, err
		}
		s := contextmgr.Summary{CurrentGoal: "build site", NextSafeAction: "continue reading", ImportantFindings: []contextmgr.Finding{{Text: "file inspected", Sources: []string{req.Materials[0].ID}}}}
		b, _ := json.Marshal(s)
		return schema.AssistantMessage(string(b), nil), nil
	}
	m.inputs = append(m.inputs, append([]*schema.Message(nil), messages...))
	m.turn++
	if m.turn <= 3 {
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: fmt.Sprint(m.turn), Function: schema.FunctionCall{Name: "inspect", Arguments: "{}"}}}}, nil
	}
	return schema.AssistantMessage("finished", nil), nil
}
func (m *compactLoopModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

type compactWrapper struct{ inner *compactLoopModel }

func (w compactWrapper) GetChatModel() model.ToolCallingChatModel { return w.inner }
func (w compactWrapper) GetModelName() string                     { return "compact-test" }
func (w compactWrapper) ContextOptions() contextmgr.Options {
	return contextmgr.Options{Window: 14000, Output: 1000, Margin: 1000}
}

func TestADKToolLoopPersistsCompactedMessages(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			m := &compactLoopModel{}
			base := NewBaseAgent(compactWrapper{m}, nil, nil, nil)
			reads := 0
			inspect, err := utils.InferTool("inspect", "inspect a file", func(context.Context, struct{}) (string, error) {
				reads++
				if reads == 3 {
					return "small follow-up result", nil
				}
				return strings.Repeat("file contents ", 340), nil
			})
			require.NoError(t, err)
			a := base.NewAdkAgent("code", "code", "Build the site", []tool.BaseTool{inspect})
			ctx := context.Background()
			if streaming {
				stream, err := base.GenerateStream(ctx, []*schema.Message{schema.UserMessage("build site")}, a)
				require.NoError(t, err)
				defer stream.Close()
				var events []string
				for {
					msg, err := stream.Recv()
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
					if kind := contextmgr.EventKind(msg); kind != "" {
						events = append(events, kind)
						require.Empty(t, msg.Content)
					}
				}
				require.Contains(t, events, "context_compacting")
				require.Contains(t, events, "context_ready")
			} else {
				_, err := base.Generate(ctx, []*schema.Message{schema.UserMessage("build site")}, a)
				require.NoError(t, err)
			}
			require.Equal(t, 4, m.turn)
			require.Equal(t, 1, m.summaries)
			// Third and fourth model calls must both contain the rewritten state.
			for _, index := range []int{2, 3} {
				encoded, _ := json.Marshal(m.inputs[index])
				require.Contains(t, string(encoded), "Working Summary")
				require.NotContains(t, string(encoded), `"tool_call_id":"1"`)
				require.Contains(t, string(encoded), "build site")
			}
		})
	}
}

func TestContextOutputReservationCapsProviderOutput(t *testing.T) {
	provider := &budgetTestModel{}
	wrapped := &budgetModel{inner: provider, outputLimit: 100}
	bound, err := wrapped.WithTools(nil)
	require.NoError(t, err)
	_, err = bound.Generate(context.Background(), nil, model.WithMaxTokens(500))
	require.NoError(t, err)
	require.EqualValues(t, 100, provider.outputLimit.Load())
	_, err = bound.Generate(context.Background(), nil, model.WithMaxTokens(50))
	require.NoError(t, err)
	require.EqualValues(t, 50, provider.outputLimit.Load())
}
