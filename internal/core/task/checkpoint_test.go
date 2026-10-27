package task

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCheckpointPrivateStateAndApprovalHash(t *testing.T) {
	cp := Checkpoint{State: json.RawMessage(`{"OriginalPrompt":"private history"}`), SnapshotPath: "private-path", ApprovedHash: "approved", Plan: Plan{Goal: "goal", Todos: []Todo{{ID: "a", Title: "generate", Status: "pending"}}}}
	data, err := cp.Encode()
	require.NoError(t, err)
	restored, err := DecodeCheckpoint(data)
	require.NoError(t, err)
	require.Equal(t, cp.State, restored.State)
	require.Equal(t, cp.SnapshotPath, restored.SnapshotPath)
	require.Equal(t, cp.ApprovedHash, restored.ApprovedHash)
	public, err := json.Marshal(restored)
	require.NoError(t, err)
	require.NotContains(t, string(public), "private history")
	require.NotContains(t, string(public), "private-path")
	require.NotContains(t, string(public), "approved")
	hash := PlanHash(cp.Plan, 1)
	require.Equal(t, "pending", cp.Plan.Todos[0].Status)
	cp.Plan.Todos[0].Status = "completed"
	require.Equal(t, hash, PlanHash(cp.Plan, 1))
	require.NotEqual(t, hash, PlanHash(cp.Plan, 2))
}
