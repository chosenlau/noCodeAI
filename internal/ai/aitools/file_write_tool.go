package aitools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type FileWriteToolParams struct {
	RelativePath string `json:"relative_path" jsonschema:"description=文件的相对路径"`
	Content      string `json:"content" jsonschema:"description=文件的写入内容"`
}

type FileWriteTool struct {
	MyBaseTool
}

func CreateFileWriteTool() (*FileWriteTool, error) {
	streamTool, err := utils.InferStreamTool("writeFile", "写入文件到指定路径", fileWriteToolFunc)
	if err != nil {
		return nil, err
	}
	return &FileWriteTool{
		MyBaseTool: MyBaseTool{
			BaseTool:    streamTool,
			displayName: "写入文件",
			toolName:    "writeFile",
		},
	}, nil
}

func fileWriteToolFunc(ctx context.Context, params FileWriteToolParams) (*schema.StreamReader[*schema.ToolResult], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relativePath := params.RelativePath
	content := params.Content
	root, path, err := openToolRoot(ctx, relativePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return journalMutation(ctx, "writeFile", params, func() (string, error) {
		if err := execution.CheckWrite(root, path, int64(len(content))); err != nil {
			return "", err
		}
		if err := root.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := root.WriteFile(path, []byte(content), 0644); err != nil {
			return "", err
		}
		return "文件写入成功: " + relativePath, nil
	})
}

func (t *FileWriteTool) GenerateToolExecutedResult(arguments string) string {
	var params FileWriteToolParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf("\n\n[工具调用] %s\n参数解析失败\n\n", t.displayName)
	}

	ext := ""
	if idx := strings.LastIndex(params.RelativePath, "."); idx != -1 {
		ext = params.RelativePath[idx+1:]
	}

	return fmt.Sprintf("\n\n[工具调用] %s %s\n```%s\n%s\n```\n\n",
		t.displayName, params.RelativePath, ext, params.Content)
}
