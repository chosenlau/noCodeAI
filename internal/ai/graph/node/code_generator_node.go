package node

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	file "github.com/chosenlau/noCodeAI/pkg/myfile"
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
	appID := input.WorkFlowContext.AppID
	msg := buildCodeGenMessages(input.WorkFlowContext)
	savePath, err := file.GetCodeOutputRoot()
	if err != nil {
		logger.Errorf("获取保存路径失败: %v", err)
		return nil, fmt.Errorf("获取保存路径失败: %w", err)
	}
	CodeGenType := input.WorkFlowContext.GenerationType

	logger.Info(fmt.Sprintf("%s代码生成", enum.CodeGenTypeTextMap[CodeGenType]))
	streamResp, err := n.CodeGenFacade.GenCodeStreamAndSave(ctx, appID, msg, CodeGenType)
	if err != nil {
		logger.Errorf("代码生成失败: %v", err)
		return nil, fmt.Errorf("代码生成失败: %w", err)
	}
	for {
		chunk, err := streamResp.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			logger.Errorf("读取代码流失败: %v", err)
			break
		}
		if input.WorkFlowContext.StreamChunkCallback != nil {
			//回调函数用于将agentturn信息返回给前端
			input.WorkFlowContext.StreamChunkCallback(chunk.Content)
		}
	}
	generatedCodeDir := filepath.Join(savePath, fmt.Sprintf("%s_%d", CodeGenType, appID))
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

	// 如果质检失败，追加错误修复提示
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

	prompt += "\n请根据上述问题和建议重新生成代码，确保修复所有提到的问题。"
	return prompt
}
