package service

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

type INoCodeAIService interface {
	GenerateHtmlCode(ctx context.Context, userMessage string) (*schema.Message, error)
	GenerateMultiFileCode(ctx context.Context, userMessage string) (*schema.Message, error)
}
