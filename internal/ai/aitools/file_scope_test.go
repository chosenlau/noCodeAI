package aitools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestFileToolsUseExecutionWorkspaceAndRejectEscapes(t *testing.T) {
	root := t.TempDir()
	ctx := execution.WithWorkspace(context.Background(), &execution.Workspace{AppID: 1, ExecutionID: "one", Root: root})
	stream, err := fileWriteToolFunc(ctx, FileWriteToolParams{RelativePath: "src/app.js", Content: "original"})
	require.NoError(t, err)
	stream.Close()
	data, err := os.ReadFile(filepath.Join(root, "src/app.js"))
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
	for _, path := range []string{"../outside", filepath.Join(t.TempDir(), "outside"), `..\outside`} {
		_, err = fileWriteToolFunc(ctx, FileWriteToolParams{RelativePath: path, Content: "bad"})
		require.Error(t, err)
		_, err = fileReadToolFunc(ctx, FileReadToolParams{RelativePath: path})
		require.Error(t, err)
		_, err = fileModifyToolFunc(ctx, FileModifyToolParams{RelativeFilePath: path, OldContent: "x", NewContent: "y"})
		require.Error(t, err)
		_, err = fileDeleteToolFunc(ctx, FileDeleteToolParams{RelativePath: path})
		require.Error(t, err)
		_, err = fileDirReadToolFunc(ctx, FileDirReadToolParams{RelativePath: path})
		require.Error(t, err)
	}
}

func TestFileToolBudgetsRejectBeforeMutation(t *testing.T) {
	root := t.TempDir()
	ctx := execution.WithWorkspace(context.Background(), &execution.Workspace{AppID: 1, ExecutionID: "budget", Root: root})
	_, err := fileWriteToolFunc(ctx, FileWriteToolParams{RelativePath: "large.txt", Content: strings.Repeat("a", int(execution.MaxFileBytes)+1)})
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(root, "large.txt"))
	ctx = execution.WithBudget(ctx, execution.NewBudget(execution.BudgetOptions{ToolCalls: 1}))
	stream, err := fileWriteToolFunc(ctx, FileWriteToolParams{RelativePath: "ok.txt", Content: "ok"})
	require.NoError(t, err)
	stream.Close()
	_, err = fileDeleteToolFunc(ctx, FileDeleteToolParams{RelativePath: "ok.txt"})
	require.ErrorIs(t, err, execution.ErrBudgetExceeded)
	require.FileExists(t, filepath.Join(root, "ok.txt"))
}

type memoryToolJournal struct {
	call    execution.ToolCall
	record  execution.ToolRecord
	claimed bool
}

func (j *memoryToolJournal) BeginTool(_ context.Context, c execution.ToolCall) (execution.ToolRecord, bool, error) {
	if j.claimed {
		if c.ArgumentsHash != j.call.ArgumentsHash {
			return execution.ToolRecord{}, false, execution.ErrToolConflict
		}
		return j.record, false, nil
	}
	j.claimed = true
	j.call = c
	j.record = execution.ToolRecord{Status: "running"}
	return j.record, true, nil
}
func (j *memoryToolJournal) FinishTool(_ context.Context, _ execution.ToolCall, r execution.ToolRecord) error {
	j.record = r
	return nil
}

func TestToolJournalUsesRealCallIDAndDoesNotRepeatDelete(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "old.txt"), []byte("old"), 0644))
	j := &memoryToolJournal{}
	ctx := execution.WithJournal(execution.WithWorkspace(context.Background(), &execution.Workspace{AppID: 1, ExecutionID: "run", Root: root}), j)
	del, err := CreateFileDeleteTool()
	require.NoError(t, err)
	node, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{Tools: []tool.BaseTool{del.BaseTool}})
	require.NoError(t, err)
	msg := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{ID: "call-1", Function: schema.FunctionCall{Name: "deleteFile", Arguments: `{"relative_path":"old.txt"}`}}}}
	stream, err := node.Stream(ctx, msg)
	require.NoError(t, err)
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
	}
	stream.Close()
	require.Equal(t, "succeeded", j.record.Status)
	require.Equal(t, "call-1", j.call.CallID)
	// Re-create the path. Replaying the same logical call must not delete it.
	require.NoError(t, os.WriteFile(filepath.Join(root, "old.txt"), []byte("new"), 0644))
	stream, err = node.Stream(ctx, msg)
	require.NoError(t, err)
	for {
		_, err = stream.Recv()
		if err != nil {
			break
		}
	}
	stream.Close()
	data, err := os.ReadFile(filepath.Join(root, "old.txt"))
	require.NoError(t, err)
	require.Equal(t, "new", string(data))
}
