package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/prompt"
	"github.com/chosenlau/noCodeAI/internal/monitor"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ImageCollectionPlanAgent struct {
	*BaseAgent
	adkAgent *adk.ChatModelAgent
}

func NewImageCollectionPlanAgent(
	chatModel ChatModelWrapperAdaptor,
	checkpointStore adk.CheckPointStore,
	metricsCollector *monitor.AiModelMetricsCollector,
) *ImageCollectionPlanAgent {
	baseAgent := NewBaseAgent(chatModel, metricsCollector, checkpointStore, nil)
	if err := prompt.LoadPrompts(); err != nil {
		return nil
	}

	adkAgent := baseAgent.NewAdkAgent(
		"图片收集计划助手",
		"根据用户需求规划图片收集任务，包括内容图片、插画、架构图和Logo",
		prompt.GetImageCollectionPlanPrompt(),
		[]tool.BaseTool{},
	)

	return &ImageCollectionPlanAgent{
		BaseAgent: baseAgent,
		adkAgent:  adkAgent,
	}
}

func (a *ImageCollectionPlanAgent) PlanImageCollection(ctx context.Context, messages []*schema.Message) (aimodel.ImageCollectionPlan, error) {
	result, err := a.Generate(ctx, messages, a.adkAgent)
	if err != nil {
		return aimodel.ImageCollectionPlan{}, err
	}

	return parseImageCollectionPlan(result.Content)
}

func parseImageCollectionPlan(content string) (aimodel.ImageCollectionPlan, error) {
	var plan aimodel.ImageCollectionPlan

	re := regexp.MustCompile("(?s)```json\\s*\\n?(.*?)\\n?```")
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		content = strings.TrimSpace(matches[1])
	} else {
		content = strings.TrimSpace(content)
		if strings.HasPrefix(content, "```json") {
			content = strings.TrimPrefix(content, "```json")
		}
		if strings.HasPrefix(content, "```") {
			content = strings.TrimPrefix(content, "```")
		}
		if strings.HasSuffix(content, "```") {
			content = strings.TrimSuffix(content, "```")
		}
		content = strings.TrimSpace(content)
	}

	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return aimodel.ImageCollectionPlan{
			ContentImageTasks: []aimodel.ImageSearchTask{},
			IllustrationTasks: []aimodel.IllustrationTask{},
			DiagramTasks:      []aimodel.DiagramTask{},
			LogoTasks:         []aimodel.LogoTask{},
		}, fmt.Errorf("解析图片收集计划失败: %w, 原始响应: %s", err, content)
	}

	return plan, nil
}
