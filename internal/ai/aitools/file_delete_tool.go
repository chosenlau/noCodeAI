package aitools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type FileDeleteToolParams struct {
	RelativePath string `json:"relative_path" jsonschema:"description=文件的相对路径"`
}

type FileDeleteTool struct {
	MyBaseTool
}

func (t *FileDeleteTool) GenerateToolExecutedResult(arguments string) string {
	var params FileDeleteToolParams
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf("\n\n[工具调用] %s\n参数解析失败\n\n", t.displayName)
	}

	return fmt.Sprintf("\n\n[工具调用] %s %s\n\n", t.displayName, params.RelativePath)
}

func CreateFileDeleteTool() (*FileDeleteTool, error) {
	streamTool, err := utils.InferStreamTool("deleteFile", "删除指定路径的文件", fileDeleteToolFunc)
	if err != nil {
		return nil, err
	}
	return &FileDeleteTool{
		MyBaseTool: MyBaseTool{
			BaseTool:    streamTool,
			displayName: "删除文件",
			toolName:    "deleteFile",
		},
	}, nil
}

func fileDeleteToolFunc(ctx context.Context, params FileDeleteToolParams) (*schema.StreamReader[*schema.ToolResult], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relativePath := params.RelativePath
	root, path, err := openToolRoot(ctx, relativePath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return journalMutation(ctx, "deleteFile", params, func() (string, error) {
		info, err := root.Stat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("deleteFile only deletes regular files")
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := root.Remove(path); err != nil {
			return "", err
		}
		return "文件删除成功: " + relativePath, nil
	})
}
