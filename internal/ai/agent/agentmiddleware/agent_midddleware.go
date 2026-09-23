package agentmiddleware

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type AgentMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

func NewAgentMiddleware(memoryStore store.MemoryStore) *AgentMiddleware {
	return &AgentMiddleware{}
}

var (
	sensitiveWords = []string{
		"忽略之前的指令", "ignore previous instructions", "ignore above",
		"破解", "hack", "绕过", "bypass", "越狱", "jailbreak",
	}

	injectionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)ignore\s+(?:previous|above|all)\s+(?:instructions?|commands?|prompts?)`),
		regexp.MustCompile(`(?i)(?:forget|disregard)\s+(?:everything|all)\s+(?:above|before)`),
		regexp.MustCompile(`(?i)(?:pretend|act|behave)\s+(?:as|like)\s+(?:if|you\s+are)`),
		regexp.MustCompile(`(?i)system\s*:\s*you\s+are`),
		regexp.MustCompile(`(?i)new\s+(?:instructions?|commands?|prompts?)\s*:`),
	}

	outputSensitiveWords = []string{
		"密码", "password", "secret", "token",
		"api key", "私钥", "证书", "credential",
	}
)

func validateInput(input string) error {
	if len(input) > 1000 {
		return errors.New("输入内容过长，不要超过 1000 字")
	}

	if strings.TrimSpace(input) == "" {
		return errors.New("输入内容不能为空")
	}

	lowerInput := strings.ToLower(input)
	for _, word := range sensitiveWords {
		if strings.Contains(lowerInput, strings.ToLower(word)) {
			return errors.New("输入包含不当内容，请修改后重试")
		}
	}

	for _, pattern := range injectionPatterns {
		if pattern.MatchString(input) {
			return errors.New("检测到恶意输入，请求被拒绝")
		}
	}

	return nil
}

func validateOutput(response string) error {
	if strings.TrimSpace(response) == "" {
		return errors.New("响应内容为空，请重新生成完整的内容")
	}

	if len(strings.TrimSpace(response)) < 10 {
		return errors.New("响应内容过短，请提供更详细的内容")
	}

	if containsSensitiveContent(response) {
		return errors.New("包含敏感信息，请重新生成内容，避免包含敏感信息")
	}

	return nil
}

func containsSensitiveContent(response string) bool {
	lowerResponse := strings.ToLower(response)
	for _, word := range outputSensitiveWords {
		if strings.Contains(lowerResponse, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

func (m *AgentMiddleware) WrapModel(
	ctx context.Context,
	chatModel model.BaseChatModel,
	mc *adk.ModelContext,
) (model.BaseChatModel, error) {
	return &innerModel{
		inner: chatModel,
	}, nil
}

type innerModel struct {
	inner model.BaseChatModel
}

func (m *innerModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	err := validateInput(msgs[len(msgs)-1].Content)
	if err != nil {
		return nil, err
	}
	resp, err := m.inner.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}

	if resp != nil {
		if err := validateOutput(resp.Content); err != nil {
			return nil, err
		}
	}

	return resp, nil
}

func (m *innerModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	err := validateInput(msgs[len(msgs)-1].Content)
	if err != nil {
		return nil, err
	}

	stream, err := m.inner.Stream(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}

	return m.wrapOutputStream(stream), nil
}

func (m *innerModel) wrapOutputStream(inner *schema.StreamReader[*schema.Message]) *schema.StreamReader[*schema.Message] {
	reader, writer := schema.Pipe[*schema.Message](2)

	go func() {
		defer writer.Close()
		var buffer string
		for {
			msg, err := inner.Recv()
			if err != nil {
				if err.Error() != "EOF" {
					writer.Send(nil, err)
				}
				return
			}
			buffer += msg.Content
			normalizeEmptyExitToolArguments(msg)
			if buffer != "" {
				// if containsSensitiveContent(buffer) {
				// 	writer.Send(nil, errors.New("检测到敏感信息，输出已中断"))
				// 	return
				// }
				writer.Send(msg, nil)
			}
		}
	}()

	return reader
}

func normalizeEmptyExitToolArguments(msg *schema.Message) {
	if msg == nil || len(msg.ToolCalls) == 0 {
		return
	}
	if msg.ResponseMeta == nil || msg.ResponseMeta.FinishReason != "tool_calls" {
		return
	}
	for index := range msg.ToolCalls {
		toolCall := &msg.ToolCalls[index]
		if toolCall.Function.Name == "exit" && strings.TrimSpace(toolCall.Function.Arguments) == "" {
			toolCall.Function.Arguments = "{}"
		}
	}
}
