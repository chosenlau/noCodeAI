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
	"github.com/cloudwego/eino/compose"
)

type ContentImageCollectorNode struct {
	cfg *config.Config
}

func NewContentImageCollectorNode(cfg *config.Config) *compose.Lambda {
	node := &ContentImageCollectorNode{cfg: cfg}
	return compose.InvokableLambda(node.execute)
}

func (n *ContentImageCollectorNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 内容图片收集")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	plan := workflowContext.ImageCollectionPlan
	imageList := n.executeContentImageTasks(plan.ContentImageTasks)

	logger.Infof("内容图片收集完成，共收集到 %d 张图片", len(imageList))

	workflowContext.ContentImage = imageList
	state.NotifyStepCompleted(workflowContext, "内容图片收集")

	return input, nil
}

func (n *ContentImageCollectorNode) executeContentImageTasks(tasks []aimodel.ImageSearchTask) []aimodel.ImageSource {
	if len(tasks) == 0 {
		return []aimodel.ImageSource{}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	collectedImages := make([]aimodel.ImageSource, 0)
	results := make(chan []*aimodel.ImageSource, len(tasks))

	for _, task := range tasks {
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
