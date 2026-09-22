package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/config"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/manager"
	"github.com/cloudwego/eino/compose"
)

type LogoCollectorNode struct {
	cfg        *config.Config
	cosManager *manager.CosManager
}

func NewLogoCollectorNode(cfg *config.Config, cosManager *manager.CosManager) *compose.Lambda {
	node := &LogoCollectorNode{
		cfg:        cfg,
		cosManager: cosManager,
	}
	return compose.InvokableLambda(node.execute)
}

func (n *LogoCollectorNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: Logo生成")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	plan := workflowContext.ImageCollectionPlan
	imageList := n.executeLogoTasks(ctx, plan.LogoTasks)

	logger.Infof("Logo生成完成，共生成 %d 张图片", len(imageList))

	workflowContext.Logos = imageList
	state.NotifyStepCompleted(workflowContext, "Logo生成")

	return input, nil
}

func (n *LogoCollectorNode) executeLogoTasks(ctx context.Context, tasks []aimodel.LogoTask) []aimodel.ImageSource {
	if len(tasks) == 0 {
		return []aimodel.ImageSource{}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	collectedImages := make([]aimodel.ImageSource, 0)
	results := make(chan []*aimodel.ImageSource, len(tasks))

	for _, task := range tasks {
		wg.Add(1)
		go func(description string) {
			defer wg.Done()
			images, err := aitools.GenerateLogos(ctx, n.cfg.DashScope.APIKey, n.cfg.DashScope.ImageModel, n.cosManager, description)
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
