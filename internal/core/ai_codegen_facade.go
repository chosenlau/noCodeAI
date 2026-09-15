package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/ai_model"
	"github.com/chosenlau/noCodeAI/internal/core/saver"

	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/schema"
)

type NoCodeAIGenFacade struct {
	codeGenFactory *agent.CodeGenAgentFactory
	codeSaver      *saver.CodeSaver
}

func NewNoCodeAIGenFacade(codeGenFactory *agent.CodeGenAgentFactory,
	codeSaver *saver.CodeSaver) *NoCodeAIGenFacade {
	return &NoCodeAIGenFacade{
		codeGenFactory: codeGenFactory,
		codeSaver:      codeSaver,
	}
}

// processCodeStream 处理代码流式数据并保存
func (y *NoCodeAIGenFacade) processCodeStream(respStream *schema.StreamReader[*schema.Message], appID int64, typeStr enum.CodeGenTypeEnum) (*schema.StreamReader[*schema.Message], error) {
	// 先复制流，一个用于处理，一个返回给上游
	streams := respStream.Copy(2)
	processingStream := streams[0]
	returnStream := streams[1]

	// 在 goroutine 中处理流数据，不阻塞返回
	go func() {
		var builder strings.Builder
		defer processingStream.Close()

		for {
			chunk, err := processingStream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
			builder.WriteString(chunk.Content)
		}

		switch typeStr {
		case enum.HtmlCodeGen:
			var result aimodel.HtmlCodeResponse
			response := agent.ParseCodeResponse([]byte(builder.String()))
			json.Unmarshal(response, &result)

			dirPath, err := y.codeSaver.SaveHtml(appID, &result)
			if err != nil {
				logger.Error("代码保存失败: %v", err)
			}
			logger.Info("代码已保存到目录: %s", dirPath)
		case enum.MultiFileGen:
			var result aimodel.MultiFileCodeResponse
			response := agent.ParseCodeResponse([]byte(builder.String()))
			json.Unmarshal(response, &result)
			dirPath, err := y.codeSaver.SaveMultiFile(appID, &result)
			if err != nil {
				logger.Error("代码保存失败: %v", err)
			}
			logger.Info("代码已保存到目录: %s", dirPath)
		default:
			logger.Errorf("不支持的代码生成类型: %s", typeStr)
		}

	}()

	return returnStream, nil
}

func (y *NoCodeAIGenFacade) GenCodeStreamAndSave(ctx context.Context, appID int64, userMessage string, typeStr enum.CodeGenTypeEnum) (*schema.StreamReader[*schema.Message], error) {
	genAgent, err := y.codeGenFactory.GetCodeGenAgent(ctx,appID, typeStr)
	if err != nil {
		return nil, err
	}
	switch typeStr {
	case enum.HtmlCodeGen:
		streamResp, err := genAgent.GenerateHtmlCodeStream(ctx, userMessage)
		if err != nil {
			return nil, err
		}
		return y.processCodeStream(streamResp, appID, typeStr)
	case enum.MultiFileGen:
		streamResp, err := genAgent.GenerateMultiFileCodeStream(ctx, userMessage)
		if err != nil {
			return nil, err
		}
		return y.processCodeStream(streamResp, appID, typeStr)
	default:
		return nil, fmt.Errorf("不支持的代码生成类型: %s", typeStr)
	}
}
