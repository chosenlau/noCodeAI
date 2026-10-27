package aitools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"strings"
)

type FileModifyToolParams struct {
	RelativeFilePath string `json:"relativeFilePath" jsonschema:"description=文件的相对路径"`
	OldContent       string `json:"oldContent" jsonschema:"description=要替换的旧内容"`
	NewContent       string `json:"newContent" jsonschema:"description=替换后的新内容"`
}

type FileModifyTool struct {
	MyBaseTool
}

func (t *FileModifyTool) GenerateToolExecutedResult(arguments string) string {
	var params FileModifyToolParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf("\n\n[工具调用] %s\n参数解析失败\n\n", t.displayName)
	}

	return fmt.Sprintf("\n\n[工具调用] %s %s\n\n替换前:\n```\n%s\n```\n\n替换后:\n```\n%s\n```\n\n",
		t.displayName, params.RelativeFilePath, params.OldContent, params.NewContent)
}

func CreateFileModifyTool() (*FileModifyTool, error) {
	streamTool, err := utils.InferStreamTool("modifyFile", "修改指定路径的文件内容，通过替换旧内容为新内容", fileModifyToolFunc)
	if err != nil {
		return nil, err
	}
	return &FileModifyTool{
		MyBaseTool: MyBaseTool{
			BaseTool:    streamTool,
			displayName: "修改文件",
			toolName:    "modifyFile",
		},
	}, nil
}

func fileModifyToolFunc(ctx context.Context, params FileModifyToolParams) (*schema.StreamReader[*schema.ToolResult], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relativeFilePath := params.RelativeFilePath
	oldContent := params.OldContent
	newContent := params.NewContent
	root, path, err := openToolRoot(ctx, relativeFilePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return journalMutation(ctx, "modifyFile", params, func() (string, error) {
		if oldContent == "" {
			return "", fmt.Errorf("oldContent must not be empty")
		}
		content, err := execution.ReadFile(root, path)
		if err != nil {
			return "", err
		}
		if !strings.Contains(string(content), oldContent) {
			return "", fmt.Errorf("old content does not match")
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		resultSize := int64(len(content)) + int64(strings.Count(string(content), oldContent))*int64(len(newContent)-len(oldContent))
		if resultSize > execution.MaxFileBytes {
			return "", fmt.Errorf("modified file exceeds size budget")
		}
		updated := strings.ReplaceAll(string(content), oldContent, newContent)
		if err := execution.CheckWrite(root, path, int64(len(updated))); err != nil {
			return "", err
		}
		if err := root.WriteFile(path, []byte(updated), 0644); err != nil {
			return "", err
		}
		return "文件修改成功: " + relativeFilePath, nil
	})
}
