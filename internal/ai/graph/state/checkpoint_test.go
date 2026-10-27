package state

import (
	"testing"

	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRestoresBusinessStateWithoutRuntimeResources(t *testing.T) {
	w := &WorkFlowContext{
		AppID: 9, OriginalPrompt: "build a page", EnhancedPrompt: "enhanced",
		ExecutionMode: ModeModify,
		CodeContent:   map[string]string{"index.html": "draft"},
		RetryCount:    1, MaxRetries: 3, GenerateCodeDir: "old-workspace",
		Plan:         task.Plan{Goal: "build a page", Todos: []task.Todo{{ID: "generate", Status: "completed"}}},
		StepCallback: func(int, string) { t.Fatal("callback must not be restored") },
	}
	data, err := w.Snapshot()
	require.NoError(t, err)
	restored, err := Restore(data)
	require.NoError(t, err)
	require.Equal(t, w.CodeContent, restored.CodeContent)
	require.Equal(t, ModeModify, restored.ExecutionMode)
	require.Equal(t, w.Plan, restored.Plan)
	require.Equal(t, w.RetryCount, restored.RetryCount)
	require.Empty(t, restored.GenerateCodeDir)
	require.Nil(t, restored.StepCallback)
	require.Nil(t, restored.StreamChunkCallback)
	restored.CodeContent["index.html"] = "new attempt"
	require.Equal(t, "draft", w.CodeContent["index.html"])
}
