package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"

	"github.com/chosenlau/noCodeAI/internal/ai/aimodel/aimessage"
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

func (y *NoCodeAIGenFacade) processCodeStream(ctx context.Context, respStream *schema.StreamReader[*schema.Message], appID int64, typeStr enum.CodeGenTypeEnum) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)

	go func() {
		var builder strings.Builder
		defer writer.Close()
		defer respStream.Close()

		for {
			if err := ctx.Err(); err != nil {
				_ = writer.Send(nil, err)
				return
			}
			chunk, err := respStream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = writer.Send(nil, err)
				return
			}
			if chunk == nil {
				continue
			}
			builder.WriteString(chunk.Content)
			if !writer.Send(chunk, nil) {
				return
			}
		}

		if err := ctx.Err(); err != nil {
			_ = writer.Send(nil, err)
			return
		}
		switch typeStr {
		case enum.HtmlCodeGen:
			var result aimodel.HtmlCodeResponse
			response := agent.ParseCodeResponse([]byte(builder.String()))
			if err := json.Unmarshal(response, &result); err != nil {
				logger.Errorf("解析 HTML 代码响应失败: %v", err)
				_ = writer.Send(nil, fmt.Errorf("解析 HTML 代码响应失败: %w", err))
				return
			}

			dirPath, err := y.codeSaver.SaveHtml(appID, &result)
			if err != nil {
				logger.Errorf("HTML code save failed: %v", err)
				_ = writer.Send(nil, fmt.Errorf("HTML code save failed: %w", err))
				return
			}
			logger.Infof("代码已保存到目录: %s", dirPath)
		case enum.MultiFileGen:
			var result aimodel.MultiFileCodeResponse
			response := agent.ParseCodeResponse([]byte(builder.String()))
			if err := json.Unmarshal(response, &result); err != nil {
				logger.Errorf("multi-file code parse failed: %v", err)
				_ = writer.Send(nil, fmt.Errorf("multi-file code parse failed: %w", err))
				return
			}
			dirPath, err := y.codeSaver.SaveMultiFile(appID, &result)
			if err != nil {
				logger.Errorf("multi-file code save failed: %v", err)
				_ = writer.Send(nil, fmt.Errorf("multi-file code save failed: %w", err))
				return
			}
			logger.Infof("代码已保存到目录: %s", dirPath)
		default:
			logger.Errorf("不支持的代码生成类型: %s", typeStr)
			_ = writer.Send(nil, fmt.Errorf("不支持的代码生成类型: %s", typeStr))
		}

	}()

	return reader, nil
}

func (y *NoCodeAIGenFacade) GenCodeStreamAndSave(ctx context.Context, appID int64, msg []*schema.Message, typeStr enum.CodeGenTypeEnum) (*schema.StreamReader[*schema.Message], error) {

	switch typeStr {
	case enum.HtmlCodeGen:
		genAgent := y.codeGenFactory.HtmlAgent
		streamResp, err := genAgent.GenerateHtmlCodeStream(ctx, msg)
		if err != nil {
			return nil, err
		}
		return y.processCodeStream(ctx, streamResp, appID, typeStr)
	case enum.MultiFileGen:
		genAgent := y.codeGenFactory.MultiFileAgent
		streamResp, err := genAgent.GenerateMultiFileCodeStream(ctx, msg)
		if err != nil {
			return nil, err
		}
		return y.processCodeStream(ctx, streamResp, appID, typeStr)
	case enum.VueCodeGen:
		genAgent := y.codeGenFactory.VueAgent
		streamResp, err := genAgent.GenerateVueProjectCodeStream(ctx, msg)
		if err != nil {
			return nil, err
		}
		return y.processVueCodeStream(ctx, streamResp)
	default:
		return nil, fmt.Errorf("unsupported code generation type: %s", typeStr)
	}
}

// toolCallBuffer
// Tool call buffer
type toolCallBuffer struct {
	ID           string
	Name         string
	Args         string
	SentRequest  bool
	SentExecuted bool
}

func (y *NoCodeAIGenFacade) processVueCodeStream(ctx context.Context, respStream *schema.StreamReader[*schema.Message]) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer writer.Close()
		defer respStream.Close()

		toolCallsBuffer := make(map[int]*toolCallBuffer)
		idToIndex := make(map[string]int)

		for {
			if err := ctx.Err(); err != nil {
				_ = writer.Send(nil, err)
				return
			}
			msg, err := respStream.Recv()
			if err != nil {
				if err == io.EOF {
					return
				}
				_ = writer.Send(nil, err)
				return
			}
			if msg == nil {
				continue
			}

			var streamMsgs []interface{}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					idx := 0
					if tc.Index != nil {
						idx = *tc.Index
					}
					if _, exists := toolCallsBuffer[idx]; !exists {
						toolCallsBuffer[idx] = &toolCallBuffer{}
					}
					buffer := toolCallsBuffer[idx]
					if tc.ID != "" {
						buffer.ID = tc.ID
						idToIndex[tc.ID] = idx
					}
					if tc.Function.Name != "" {
						buffer.Name = tc.Function.Name
					}
					buffer.Args += tc.Function.Arguments
					if buffer.ID != "" && buffer.Name != "" && isValidJSON(buffer.Args) && !buffer.SentRequest {
						streamMsgs = append(streamMsgs, aimessage.NewToolRequestMessage(idx, buffer.ID, buffer.Name, buffer.Args))
						buffer.SentRequest = true
					}
				}
			} else if msg.Role == schema.Tool {
				toolCallID := msg.ToolCallID
				arguments := ""
				if idx, exists := idToIndex[toolCallID]; exists {
					if buffer, ok := toolCallsBuffer[idx]; ok {
						arguments = buffer.Args
						if !buffer.SentRequest {
							streamMsgs = append(streamMsgs, aimessage.NewToolRequestMessage(idx, buffer.ID, buffer.Name, buffer.Args))
							buffer.SentRequest = true
						}
					}
					delete(toolCallsBuffer, idx)
					delete(idToIndex, toolCallID)
				}
				streamMsgs = append(streamMsgs, aimessage.NewToolExecutedMessage(0, msg.ToolCallID, msg.ToolName, arguments, msg.Content))
			} else if msg.Content != "" {
				streamMsgs = append(streamMsgs, aimessage.NewAIResponseMessage(msg.Content))
			}

			for _, streamMsg := range streamMsgs {
				if err := ctx.Err(); err != nil {
					_ = writer.Send(nil, err)
					return
				}
				if streamMsg == nil {
					continue
				}
				msgBytes, err := json.Marshal(streamMsg)
				if err != nil {
					logger.Errorf("failed to serialize stream message: %v", err)
					continue
				}
				if !writer.Send(&schema.Message{Content: string(msgBytes)}, nil) {
					return
				}
			}
		}
	}()
	return reader, nil
}

// isValidJSON
func isValidJSON(s string) bool {
	if s == "" {
		return false
	}
	var js interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
