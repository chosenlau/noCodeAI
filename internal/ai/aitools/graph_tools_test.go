package aitools

import (
	"context"
	"fmt"
	"testing"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/dal"
	"github.com/chosenlau/noCodeAI/internal/manager"
	"github.com/cloudwego/hertz/pkg/common/test/assert"
)

func TestCreateImageSearchTool(t *testing.T) {
	initConfig := config.InitConfig()
	images, err := SearchImages(initConfig.Pexels.APIKey, "宠物")
	if err != nil {
		fmt.Println(err)
	}
	for _, image := range images {
		fmt.Println(image)
	}
	assert.NotNil(t, images)
}

func TestCreateUndrawIllustrationTool(t *testing.T) {
	undrawIllustrations, err := SearchUndrawIllustrations("sad")
	if err != nil {
		fmt.Println(err)
	}
	for _, undrawIllustration := range undrawIllustrations {
		fmt.Println(undrawIllustration)
	}
	assert.NotNil(t, undrawIllustrations)
}

func TestCreateMermaidDiagramTool(t *testing.T) {
	initConfig := config.InitConfig()
	cosClient := dal.InitCOSClient(initConfig)
	cosManager := manager.NewCosManager(cosClient, initConfig)
	ctx := context.Background()
	mermaidDiagram, err := GenerateMermaidDiagram(ctx, cosManager, "flowchart LR\n"+
		"Start([开始]) --> Input[输入数据]\n"+
		"Input --> Process[处理数据]\n"+
		"Process --> Decision{是否有效?}\n"+
		"Decision -->|是| Output[输出结果]\n"+
		"Decision -->|否| Error[错误处理]\n"+
		"Output --> End([结束])\n"+
		"Error --> End", "简单系统架构图")
	if err != nil {
		fmt.Println(err)
	}
	assert.NotNil(t, mermaidDiagram)
	fmt.Println(mermaidDiagram[0])
}

func TestCreateLogoGeneratorTool(t *testing.T) {
	initConfig := config.InitConfig()
	client := dal.InitCOSClient(initConfig)
	cosManager := manager.NewCosManager(client, initConfig)

	logos, err := GenerateLogos(context.Background(), initConfig.DashScope.APIKey, initConfig.DashScope.ImageModel, cosManager, "技术公司现代简约风格logo")
	if err != nil {
		fmt.Println(err)
	}
	for _, logo := range logos {
		fmt.Println(logo)
	}
}
