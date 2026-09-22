package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/gopkg/util/logger"
	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/aitools"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/manager"
	"github.com/cloudwego/eino/compose"
)

type DiagramCollectorNode struct {
	cosManager *manager.CosManager
}

func NewDiagramCollectorNode(cosManager *manager.CosManager) *compose.Lambda {
	node := &DiagramCollectorNode{cosManager: cosManager}
	return compose.InvokableLambda(node.execute)
}

func (n *DiagramCollectorNode) execute(ctx context.Context, input *state.GraphState) (*state.GraphState, error) {
	logger.Info("执行节点: 架构图生成")

	workflowContext := input.WorkFlowContext
	if workflowContext == nil {
		return nil, fmt.Errorf("WorkFlowContext 为空")
	}

	plan := workflowContext.ImageCollectionPlan
	imageList := n.executeDiagramTasks(ctx, plan.DiagramTasks)

	logger.Infof("架构图生成完成，共生成 %d 张图片", len(imageList))

	workflowContext.Diagrams = imageList
	state.NotifyStepCompleted(workflowContext, "架构图生成")

	return input, nil
}

func (n *DiagramCollectorNode) executeDiagramTasks(ctx context.Context, tasks []aimodel.DiagramTask) []aimodel.ImageSource {
	if len(tasks) == 0 {
		return []aimodel.ImageSource{}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	collectedImages := make([]aimodel.ImageSource, 0)
	results := make(chan []*aimodel.ImageSource, len(tasks))

	for _, task := range tasks {
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
