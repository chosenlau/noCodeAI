package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type CodeGeneratorNode struct {
	CodeGenFacade *core.NoCodeAIGenFacade
}

func NewCodeGeneratorNode(CodeGenFacade *core.NoCodeAIGenFacade) *compose.Lambda {
	node := &CodeGeneratorNode{
		CodeGenFacade: CodeGenFacade,
	}
	return compose.InvokableLambda(node.execute)
}

func (n *CodeGeneratorNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	state.NotifyStepStart(input.WorkFlowContext, "代码生成")
	appID := input.WorkFlowContext.AppID
	msg := buildCodeGenMessages(input.WorkFlowContext)
	CodeGenType := input.WorkFlowContext.GenerationType

	logger.Info(fmt.Sprintf("%s代码生成", enum.CodeGenTypeTextMap[CodeGenType]))
	toolCtx := context.WithValue(ctx, "appId", appID)
	var streamResp *schema.StreamReader[*schema.Message]
	var err error
	if input.WorkFlowContext.ExecutionMode == state.ModeModify || isQualityCheckFailed(input.WorkFlowContext.QualityResult) {
		streamResp, err = n.CodeGenFacade.ModifyCodeStream(toolCtx, appID, msg)
	} else {
		streamResp, err = n.CodeGenFacade.GenCodeStreamAndSave(toolCtx, appID, msg, CodeGenType)
	}
	if err != nil {
		logger.Errorf("代码生成失败: %v", err)
		return nil, fmt.Errorf("代码生成失败: %w", err)
	}
	defer streamResp.Close()
	for {
		chunk, err := streamResp.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("code generation stream read failed: %w", err)
		}
		if input.WorkFlowContext.StreamChunkCallback != nil {
			input.WorkFlowContext.StreamChunkCallback(chunk.Content)
		}
	}

	generatedCodeDir, err := n.CodeGenFacade.SavedCodeDirContext(ctx, CodeGenType, appID)
	if err != nil {
		return nil, fmt.Errorf("resolve generated code directory: %w", err)
	}
	if _, err := os.Stat(generatedCodeDir); err != nil {
		return nil, fmt.Errorf("generated code directory is missing: %w", err)
	}
	logger.Infof("AI 代码生成完成，生成目录: %s", generatedCodeDir)
	input.WorkFlowContext.GenerateCodeDir = generatedCodeDir
	state.NotifyStepCompleted(input.WorkFlowContext, "代码生成")

	return input, nil
}

func buildCodeGenMessages(workflowContext *state.WorkFlowContext) []*schema.Message {
	var messages []*schema.Message

	// 基础提示词
	userPrompt := workflowContext.EnhancedPrompt
	if userPrompt == "" {
		userPrompt = workflowContext.OriginalPrompt
	}
	userPrompt += fmt.Sprintf("\n\n本项目代码类型：%s。保留该技术栈。", workflowContext.GenerationType)
	if workflowContext.Plan.Goal != "" {
		plan, _ := json.Marshal(workflowContext.Plan)
		userPrompt += "\n\n## 执行计划与剩余任务\n" + string(plan)
	}
	if isQualityCheckFailed(workflowContext.QualityResult) {
		userPrompt += buildErrorFixPrompt(workflowContext.QualityResult)
	}

	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: userPrompt,
	})

	// 如果有图片列表，追加图片信息
	if workflowContext.ImageListStr != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.User,
			Content: fmt.Sprintf("\n\n可用图片资源：\n%s", workflowContext.ImageListStr),
		})
	}
	return messages
}

func isQualityCheckFailed(qualityResult aimodel.QualityResult) bool {
	return !qualityResult.IsValid && len(qualityResult.Errors) > 0
}

func buildErrorFixPrompt(qualityResult aimodel.QualityResult) string {
	prompt := "\n\n## 上次生成的代码存在以下问题，请修复：\n"
	for _, err := range qualityResult.Errors {
		prompt += fmt.Sprintf("- %s\n", err)
	}
	if len(qualityResult.Suggestions) > 0 {
		prompt += "\n## 修复建议：\n"
		for _, suggestion := range qualityResult.Suggestions {
			prompt += fmt.Sprintf("- %s\n", suggestion)
		}
	}
	prompt += "\n请先读取当前草稿，只修复上述问题，保留已经正确的代码和功能，不要重新生成整个项目。"
	return prompt
}
