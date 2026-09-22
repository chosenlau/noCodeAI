package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/cloudwego/eino/compose"
)

type IllustrationCollectorNode struct{}

func NewIllustrationCollectorNode() *compose.Lambda {
	node := &IllustrationCollectorNode{}
	return compose.InvokableLambda(node.execute)
}

func (n *IllustrationCollectorNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 插画收集")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	plan := workflowContext.ImageCollectionPlan
	imageList := n.executeIllustrationTasks(plan.IllustrationTasks)

	logger.Infof("插画收集完成，共收集到 %d 张图片", len(imageList))

	workflowContext.Illustrations = imageList
	state.NotifyStepCompleted(workflowContext, "插画收集")

	return input, nil
}

func (n *IllustrationCollectorNode) executeIllustrationTasks(tasks []aimodel.IllustrationTask) []aimodel.ImageSource {
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
			images, err := aitools.SearchUndrawIllustrations(query)
			if err != nil {
				logger.Errorf("插画搜索失败: %v", err)
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
