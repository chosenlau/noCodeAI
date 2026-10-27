package aitools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/pkg/myfile"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func openToolRoot(ctx context.Context, name string) (*os.Root, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if budget := execution.GetBudget(ctx); budget != nil {
		if err := budget.ConsumeTool(); err != nil {
			return nil, "", err
		}
	}
	rel, err := execution.RelativePath(name)
	if err != nil {
		return nil, "", err
	}
	var root string
	if w, ok := execution.WorkspaceFromContext(ctx); ok {
		if id, ok := ctx.Value("appId").(int64); ok && id != w.AppID {
			return nil, "", fmt.Errorf("workspace app mismatch")
		}
		root = w.Root
	} else {
		id, ok := ctx.Value("appId").(int64)
		if !ok || id <= 0 {
			return nil, "", fmt.Errorf("project execution context is missing")
		}
		base, err := myfile.GetCodeOutputRoot()
		if err != nil {
			return nil, "", err
		}
		root = filepath.Join(base, fmt.Sprintf("vue_project_%d", id))
		if err := os.MkdirAll(root, 0755); err != nil {
			return nil, "", err
		}
	}
	r, err := os.OpenRoot(root)
	return r, rel, err
}

// journalMutation wraps the actual side effect, not its UI event. Uncertain
// outcomes stop replay instead of repeating a potentially completed mutation.
func journalMutation(ctx context.Context, name string, params any, fn func() (string, error)) (*schema.StreamReader[*schema.ToolResult], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j := execution.JournalFromContext(ctx)
	w, scoped := execution.WorkspaceFromContext(ctx)
	var call execution.ToolCall
	if j != nil && scoped {
		id := compose.GetToolCallID(ctx)
		if id == "" {
			return nil, fmt.Errorf("tool call ID is missing")
		}
		args, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(args)
		call = execution.ToolCall{ExecutionID: w.ExecutionID, CallID: id, ToolName: name, ArgumentsHash: hex.EncodeToString(hash[:])}
		record, claimed, err := j.BeginTool(ctx, call)
		if err != nil {
			return nil, err
		}
		if !claimed {
			if record.Status == "succeeded" {
				return textToolResult(record.Result), nil
			}
			return nil, fmt.Errorf("tool %s has %s outcome; automatic replay refused", id, record.Status)
		}
	}
	if scoped {
		unlock := w.Lock()
		defer unlock()
	}
	result, err := fn()
	if j != nil && scoped {
		record := execution.ToolRecord{Status: "succeeded", Result: result}
		if err != nil {
			record.Status = "uncertain"
			record.Error = err.Error()
		}
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		persistErr := j.FinishTool(persistCtx, call, record)
		cancel()
		if persistErr != nil {
			return nil, fmt.Errorf("persist tool outcome: %w", persistErr)
		}
	}
	if err != nil {
		return nil, err
	}
	return textToolResult(result), nil
}
func textToolResult(text string) *schema.StreamReader[*schema.ToolResult] {
	return schema.StreamReaderFromArray([]*schema.ToolResult{{Parts: []schema.ToolOutputPart{{Type: schema.ToolPartTypeText, Text: text}}}})
}
