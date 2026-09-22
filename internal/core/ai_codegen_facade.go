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

// processCodeStream 处理代码流式数据并保存
func (y *NoCodeAIGenFacade) processCodeStream(respStream *schema.StreamReader[*schema.Message], appID int64, typeStr enum.CodeGenTypeEnum) (*schema.StreamReader[*schema.Message], error) {
	// 先复制流，一个用于处理，一个返回给上游
	reader, writer := schema.Pipe[*schema.Message](2)

	// 在 goroutine 中处理流数据，不阻塞返回
	go func() {
		var builder strings.Builder
		defer writer.Close()

		for {
			chunk, err := respStream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				writer.Send(nil, err)
				return
			}
			if chunk == nil {
				continue
			}
			builder.WriteString(chunk.Content)
			writer.Send(chunk, nil)
		}

		switch typeStr {
		case enum.HtmlCodeGen:
			var result aimodel.HtmlCodeResponse
			response := agent.ParseCodeResponse([]byte(builder.String()))
			if err := json.Unmarshal(response, &result); err != nil {
				logger.Errorf("解析 HTML 代码响应失败: %v", err)
				return
			}

			dirPath, err := y.codeSaver.SaveHtml(appID, &result)
			if err != nil {
				logger.Errorf("代码保存失败: %v", err)
				return
			}
			logger.Infof("代码已保存到目录: %s", dirPath)
		case enum.MultiFileGen:
			var result aimodel.MultiFileCodeResponse
			response := agent.ParseCodeResponse([]byte(builder.String()))
			if err := json.Unmarshal(response, &result); err != nil {
				logger.Errorf("解析多文件代码响应失败: %v", err)
				return
			}
			dirPath, err := y.codeSaver.SaveMultiFile(appID, &result)
			if err != nil {
				logger.Errorf("代码保存失败: %v", err)
				return
			}
			logger.Infof("代码已保存到目录: %s", dirPath)
		default:
			logger.Errorf("不支持的代码生成类型: %s", typeStr)
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
		return y.processCodeStream(streamResp, appID, typeStr)
	case enum.MultiFileGen:
		genAgent := y.codeGenFactory.MultiFileAgent
		streamResp, err := genAgent.GenerateMultiFileCodeStream(ctx, msg)
		if err != nil {
			return nil, err
		}
		return y.processCodeStream(streamResp, appID, typeStr)
	case enum.VueCodeGen:
		genAgent := y.codeGenFactory.VueAgent
		streamResp, err := genAgent.GenerateVueProjectCodeStream(ctx, msg)
		if err != nil {
			return nil, err
		}
		return y.processVueCodeStream(streamResp)
	default:
		return nil, fmt.Errorf("不支持的代码生成类型: %s", typeStr)
	}
}

func (y *NoCodeAIGenFacade) processVueCodeStream(respStream *schema.StreamReader[*schema.Message]) (*schema.StreamReader[*schema.Message], error) {
	// 1. 创建通道流
	reader, writer := schema.Pipe[*schema.Message](2)

	// 2. 异步写入通道流
	go func() {
		defer writer.Close()

		// 初始化工具响应缓存 map
		toolCallsBuffer := make(map[int]*toolCallBuffer)
		idToIndex := make(map[string]int)

		for {
			// 消费流
			msg, err := respStream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				writer.Send(nil, err)
				return
			}

			if msg == nil {
				continue
			}

			// 💡 修改点 1：使用 Slice 来收集当前循环产生的所有消息
			// 因为在某些边界情况下，一次接收可能触发多条前端通知
			var streamMsgs []interface{}

			if len(msg.ToolCalls) > 0 {
				// 判断是工具请求类型信息 (流式片段组装)
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

					// ⚠️ 警告：依赖 isValidJSON 判断流式参数是否结束依然有风险
					// 理想情况下，应依赖 msg.ResponseMeta.FinishReason == "tool_calls" 来判定
					if buffer.ID != "" && buffer.Name != "" && isValidJSON(buffer.Args) && !buffer.SentRequest {
						streamMsgs = append(streamMsgs, aimessage.NewToolRequestMessage(idx, buffer.ID, buffer.Name, buffer.Args))
						buffer.SentRequest = true
					}
				}
			} else if msg.Role == schema.Tool {
				// 工具执行完毕，返回结果
				toolCallID := msg.ToolCallID
				arguments := ""

				if idx, exists := idToIndex[toolCallID]; exists {
					if buffer, ok := toolCallsBuffer[idx]; ok {
						arguments = buffer.Args

						// 💡 修改点 2：安全兜底
						// 如果之前 isValidJSON 没拦截成功，在工具真正执行完时，强制补发一条 ToolRequest
						if !buffer.SentRequest {
							streamMsgs = append(streamMsgs, aimessage.NewToolRequestMessage(idx, buffer.ID, buffer.Name, buffer.Args))
							buffer.SentRequest = true
						}
					}
					// 清理缓存
					delete(toolCallsBuffer, idx)
					delete(idToIndex, toolCallID)
				}
				streamMsgs = append(streamMsgs, aimessage.NewToolExecutedMessage(0, msg.ToolCallID, msg.ToolName, arguments, msg.Content))

			} else if msg.Content != "" {
				// 判断是 AI 普通文本响应类型信息
				streamMsgs = append(streamMsgs, aimessage.NewAIResponseMessage(msg.Content))
			}

			// 💡 修改点 3：遍历发送所有收集到的消息
			for _, sMsg := range streamMsgs {
				if sMsg != nil {
					msgBytes, err := json.Marshal(sMsg)
					if err != nil {
						logger.Errorf("序列化消息失败: %v", err)
						continue // 序列化失败跳过该条，不影响主流程
					}

					newMsg := &schema.Message{
						Content: string(msgBytes), // 包装成下游统一格式
					}
					writer.Send(newMsg, nil)
				}
			}
		}
	}()

	return reader, nil
}

// toolCallBuffer
// 工具信息缓存
type toolCallBuffer struct {
	ID           string
	Name         string
	Args         string
	SentRequest  bool
	SentExecuted bool
}

// isValidJSON
// 校验json格式完整性（工具流式输出的json串不完整，用于校验参数）
func isValidJSON(s string) bool {
	if s == "" {
		return false
	}
	var js interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}
