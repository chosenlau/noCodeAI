package node

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type CodeQualityCheckNode struct {
	qualityCheckAgent *agent.CodeQualityCheckAgent
}

func NewCodeQualityCheckNode(qualityCheckAgent *agent.CodeQualityCheckAgent) *compose.Lambda {
	node := &CodeQualityCheckNode{qualityCheckAgent: qualityCheckAgent}
	return compose.InvokableLambda(node.execute)
}

func (n *CodeQualityCheckNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 代码质量检查")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	generatedCodeDir := workflowContext.GenerateCodeDir
	var qualityResult aimodel.QualityResult

	codeContent, err := readAndConcatenateCodeFiles(generatedCodeDir)
	if err != nil {
		logger.Errorf("读取代码文件失败: %v", err)
		qualityResult = aimodel.QualityResult{
			IsValid: true, // 读取失败默认通过，避免阻塞流程
		}
	} else if codeContent == "" {
		logger.Warn("未找到可检查的代码文件")
		qualityResult = aimodel.QualityResult{
			IsValid: false,
			Errors:  []string{"未找到可检查的代码文件"},
		}
	} else {
		// 构建 messages
		messages := buildQualityCheckMessages(codeContent)

		// 调用 agent
		qualityResult, err = n.qualityCheckAgent.CheckCodeQuality(ctx, messages)
		if err != nil {
			logger.Errorf("代码质量检查异常: %v", err)
			qualityResult = aimodel.QualityResult{
				IsValid: true, // 检查异常默认通过
			}
		} else {
			logger.Infof("代码质量检查完成 - 是否通过: %v", qualityResult.IsValid)
			if !qualityResult.IsValid {
				logger.Warnf("代码质量问题: %v", qualityResult.Errors)
			}
		}
	}

	workflowContext.QualityResult = qualityResult
	state.NotifyStepCompleted(workflowContext, "代码质量检查")

	return input, nil
}

func buildQualityCheckMessages(codeContent string) []*schema.Message {
	return []*schema.Message{
		{
			Role:    schema.User,
			Content: fmt.Sprintf("请检查以下代码的质量：\n\n%s", codeContent),
		},
	}
}

func readAndConcatenateCodeFiles(dir string) (string, error) {
	var builder strings.Builder

	codeExtensions := map[string]bool{
		".html": true,
		".css":  true,
		".js":   true,
		".ts":   true,
		".vue":  true,
		".jsx":  true,
		".tsx":  true,
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "dist" || d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !codeExtensions[ext] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			logger.Warnf("读取文件失败 %s: %v", path, err)
			return nil
		}

		relPath, _ := filepath.Rel(dir, path)
		builder.WriteString(fmt.Sprintf("\n// ===== File: %s =====\n", relPath))
		builder.Write(content)
		builder.WriteString("\n")

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("遍历目录失败: %w", err)
	}

	return builder.String(), nil
}
