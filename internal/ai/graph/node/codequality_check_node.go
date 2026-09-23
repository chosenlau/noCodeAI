package node

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

var codeFileExtensions = map[string]bool{
	".html": true,
	".css":  true,
	".js":   true,
	".ts":   true,
	".vue":  true,
	".jsx":  true,
	".tsx":  true,
	".json": true,
}

type CodeQualityCheckNode struct {
	qualityCheckAgent *agent.CodeQualityCheckAgent
}

func NewCodeQualityCheckNode(qualityCheckAgent *agent.CodeQualityCheckAgent) *compose.Lambda {
	node := &CodeQualityCheckNode{qualityCheckAgent: qualityCheckAgent}
	return compose.InvokableLambda(node.execute)
}

func (n *CodeQualityCheckNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	if input == nil {
		return nil, fmt.Errorf("GraphState is nil")
	}
	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext is nil")
	}
	state.NotifyStepStart(workflowContext, "代码质量检查")
	codeFiles, description, err := ReadCodeFiles(workflowContext.GenerateCodeDir)

	if err != nil {
		logger.Errorf("failed to read generated code: %v", err)
		workflowContext.CodeContent = map[string]string{}
		workflowContext.QualityResult = aimodel.QualityResult{
			IsValid: false,
			Errors:  []string{fmt.Sprintf("failed to read generated code: %v", err)},
		}
	} else if len(codeFiles) == 0 {
		workflowContext.CodeContent = map[string]string{}
		workflowContext.QualityResult = aimodel.QualityResult{
			IsValid: false,
			Errors:  []string{"no code files found"},
		}
	} else {
		workflowContext.CodeContent = codeFiles
		codeText := concatenateCodeFiles(codeFiles)
		messages := buildQualityCheckMessages(workflowContext, codeText)

		if n.qualityCheckAgent == nil {
			workflowContext.QualityResult = aimodel.QualityResult{IsValid: true}
		} else {
			qualityResult, err := n.qualityCheckAgent.CheckCodeQuality(ctx, messages)
			if err != nil {
				logger.Errorf("code quality check failed: %v", err)
				qualityResult = aimodel.QualityResult{
					IsValid: false,
					Errors:  []string{err.Error()},
				}
			}
			workflowContext.QualityResult = qualityResult
			if !qualityResult.IsValid {
				logger.Warnf("code quality issues: %v", qualityResult.Errors)
			}
		}
	}
	workflowContext.Description = description
	state.NotifyStepCompleted(workflowContext, "代码质量检查")
	return input, nil
}

func buildQualityCheckMessages(workflowContext *state.WorkFlowContext, codeContent string) []*schema.Message {
	retryCount := workflowContext.RetryCount
	maxRetries := workflowContext.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return []*schema.Message{{
		Role: schema.User,
		Content: fmt.Sprintf(
			"请检查以下代码的质量。\n当前已重试次数：%d，最大重试次数：%d。请据此判断问题优先级，并尽量给出能在本次修复中完成的建议。\n\n项目描述：\n%s\n\n代码：\n%s",
			retryCount,
			maxRetries,
			workflowContext.Description,
			codeContent,
		),
	}}
}

func ReadCodeFiles(dir string) (map[string]string, string, error) {
	files := make(map[string]string)
	description := ""
	if dir == "" {
		return files, description, nil
	}

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case "node_modules", "dist", ".git":
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "description.md") {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Warnf("failed to read project description %s: %v", path, readErr)
			} else {
				description = string(content)
			}
			return nil
		}
		if !codeFileExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			logger.Warnf("failed to read code file %s: %v", path, err)
			return nil
		}
		relativePath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relativePath)] = string(content)
		return nil
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to walk code directory: %w", err)
	}
	return files, description, nil
}

func concatenateCodeFiles(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var builder strings.Builder
	for _, path := range paths {
		builder.WriteString(fmt.Sprintf("\n// ===== File: %s =====\n", path))
		builder.WriteString(files[path])
		builder.WriteString("\n")
	}
	return builder.String()
}
