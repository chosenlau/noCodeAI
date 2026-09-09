package service

import (
	"context"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/ai_model"
	"github.com/cloudwego/eino/schema"
)

type INoCodeAIService interface {
	GenerateHtmlCode(ctx context.Context, userMessage string) (*aimodel.HtmlCodeResponse, error)
	GenerateMultiFileCode(ctx context.Context, userMessage string) (*aimodel.MultiFileCodeResponse, error)
	GenerateHtmlCodeStream(ctx context.Context, userMessage string) (*schema.StreamReader[*schema.Message], error)
	GenerateMultiFileCodeStream(ctx context.Context, userMessage string) (*schema.StreamReader[*schema.Message], error)
}
