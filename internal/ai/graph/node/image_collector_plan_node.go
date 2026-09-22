package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/manager"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ImageCollectorPlanNode struct {
	planAgent  *agent.ImageCollectionPlanAgent
	cfg        *config.Config
	cosManager *manager.CosManager
}

func NewImageCollectorPlanNode(
	planAgent *agent.ImageCollectionPlanAgent,
	cfg *config.Config,
	cosManager *manager.CosManager,
) *compose.Lambda {
	node := &ImageCollectorPlanNode{
		planAgent:  planAgent,
		cfg:        cfg,
		cosManager: cosManager,
	}
	return compose.InvokableLambda(node.execute)
}

func (n *ImageCollectorPlanNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 图片收集计划")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	originalPrompt := workflowContext.OriginalPrompt

	// 构建 messages
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: originalPrompt,
		},
	}

	var imageList []aimodel.ImageSource

	plan, err := n.planAgent.PlanImageCollection(ctx, messages)
	if err != nil {
		logger.Errorf("获取图片收集计划失败: %v", err)
		imageList = []aimodel.ImageSource{}
	} else {
		logger.Info("获取到图片收集计划，开始并发执行")
		imageList = n.executeImageCollectionPlan(ctx,plan)
		logger.Infof("并发图片收集完成，共收集到 %d 张图片", len(imageList))
	}

	workflowContext.ImageList = imageList
	state.NotifyStepCompleted(workflowContext, "图片收集")

	return input, nil
}

func (n *ImageCollectorPlanNode) executeImageCollectionPlan(ctx context.Context, plan aimodel.ImageCollectionPlan) []aimodel.ImageSource {
	var wg sync.WaitGroup
	var mu sync.Mutex
	collectedImages := make([]aimodel.ImageSource, 0)

	totalTasks := len(plan.ContentImageTasks) + len(plan.IllustrationTasks) +
		len(plan.DiagramTasks) + len(plan.LogoTasks)

	if totalTasks == 0 {
		return collectedImages
	}

	results := make(chan []*aimodel.ImageSource, totalTasks)

	// 内容图片搜索任务
	for _, task := range plan.ContentImageTasks {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			images, err := aitools.SearchImages(n.cfg.Pexels.APIKey, query)
			if err != nil {
				logger.Errorf("内容图片搜索失败: %v", err)
				return
			}
			results <- images
		}(task.Query)
	}

	// 插画搜索任务
	for _, task := range plan.IllustrationTasks {
		wg.Add(1)
		go func(query string) {
			defer wg.Done()
			images, err := aitools.SearchUndrawIllustrations(query)
			if err != nil {
				logger.Errorf("插画搜索失败: %v", err)
				return
			}
			results <- images
		}(task.Query)
	}

	// 架构图生成任务
	for _, task := range plan.DiagramTasks {
		wg.Add(1)
		go func(mermaidCode, description string) {
			defer wg.Done()
			images, err := aitools.GenerateMermaidDiagram(ctx,n.cosManager, mermaidCode, description)
			if err != nil {
				logger.Errorf("架构图生成失败: %v", err)
				return
			}
			results <- images
		}(task.MermaidCode, task.Description)
	}

	// Logo生成任务
	for _, task := range plan.LogoTasks {
		wg.Add(1)
		go func(description string) {
			defer wg.Done()
			images, err := aitools.GenerateLogos(ctx,n.cfg.DashScope.APIKey, n.cfg.DashScope.ImageModel, n.cosManager, description)
			if err != nil {
				logger.Errorf("Logo生成失败: %v", err)
				return
			}
			results <- images
		}(task.Description)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for images := range results {
		mu.Lock()
		for _, img := range images {
			if img != nil {
				collectedImages = append(collectedImages, *img)
			}
		}
		mu.Unlock()
	}

	return collectedImages
}
