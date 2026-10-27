package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

// A deterministic model drives the real ADK runner, tools and facade stream.
type modificationModel struct {
	step           int
	sawRead        bool
	sawInstruction bool
}

func (m *modificationModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}
func (m *modificationModel) GetChatModel() model.ToolCallingChatModel { return m }
func (m *modificationModel) GetModelName() string                     { return "test-modify" }
func (m *modificationModel) Generate(ctx context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	for _, msg := range messages {
		if msg.Role == schema.System && strings.Contains(msg.Content, agent.ProjectModifyInstruction) {
			m.sawInstruction = true
		}
		if msg.Role == schema.Tool && strings.Contains(msg.Content, "old-title") {
			m.sawRead = true
		}
	}
	m.step++
	if m.step > 3 {
		return &schema.Message{Role: schema.Assistant, Content: "已更新标题"}, nil
	}
	name, args := "readDir", `{"relative_path":"."}`
	if m.step == 2 {
		name, args = "readFile", `{"relative_path":"index.html"}`
	}
	if m.step == 3 {
		name, args = "modifyFile", `{"relativeFilePath":"index.html","oldContent":"old-title","newContent":"new-title"}`
	}
	index := 0
	return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{Index: &index, ID: fmt.Sprintf("call-%d", m.step), Type: "function", Function: schema.FunctionCall{Name: name, Arguments: args}}}}, nil
}
func (m *modificationModel) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func TestModifyAndQualityRepairUseExistingFileTools(t *testing.T) {
	for _, codeType := range []enum.CodeGenTypeEnum{enum.HtmlCodeGen, enum.MultiFileGen, enum.VueCodeGen} {
		for _, repair := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/repair=%t", codeType, repair), func(t *testing.T) {
				root := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>old-title</h1>"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(root, "unrelated.css"), []byte("keep-me"), 0600))
				tools, err := aitools.NewToolManager()
				require.NoError(t, err)
				m := &modificationModel{}
				vue := agent.NewVueCodeGenAgent(m, nil, nil, tools, nil)
				require.NotNil(t, vue)
				facade := core.NewNoCodeAIGenFacade(agent.NewCodeGenAgentFactory(nil, nil, vue), nil)
				wc := &state.WorkFlowContext{AppID: 41, GenerationType: codeType, ExecutionMode: state.ModeModify, OriginalPrompt: "修改标题"}
				if repair {
					wc.ExecutionMode = state.ModeBootstrap
					wc.QualityResult = aimodel.QualityResult{Errors: []string{"修复标题"}}
				}
				var chunks strings.Builder
				wc.StreamChunkCallback = func(s string) { chunks.WriteString(s) }
				ctx := execution.WithWorkspace(context.Background(), &execution.Workspace{AppID: 41, ExecutionID: "test", Root: root})
				_, err = (&CodeGeneratorNode{CodeGenFacade: facade}).execute(ctx, &state.GraphState{WorkFlowContext: wc})
				require.NoError(t, err)
				require.True(t, m.sawRead)
				require.True(t, m.sawInstruction)
				data, err := os.ReadFile(filepath.Join(root, "index.html"))
				require.NoError(t, err)
				require.Equal(t, "<h1>new-title</h1>", string(data))
				data, err = os.ReadFile(filepath.Join(root, "unrelated.css"))
				require.NoError(t, err)
				require.Equal(t, "keep-me", string(data))
				require.Contains(t, chunks.String(), "tool_request")
				require.Contains(t, chunks.String(), "tool_executed")
			})
		}
	}
}

func TestModifySkipsRoutingAndBindsPlanToMode(t *testing.T) {
	wc := &state.WorkFlowContext{GenerationType: enum.HtmlCodeGen, ExecutionMode: state.ModeModify, OriginalPrompt: "修改标题"}
	input := &state.GraphState{WorkFlowContext: wc}
	_, err := (&RouterNode{}).execute(context.Background(), input)
	require.NoError(t, err)
	_, err = (&PromptEnhancerNode{}).execute(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, state.ModeModify, wc.Plan.ExecutionMode)
	hash := task.PlanHash(wc.Plan, 1)
	wc.Plan.ExecutionMode = state.ModeBootstrap
	require.NotEqual(t, hash, task.PlanHash(wc.Plan, 1))
}
