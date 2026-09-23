package core

import (
	"context"
	"testing"

	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestProcessCodeStreamStopsWhenCancelled(t *testing.T) {
	respStream := schema.StreamReaderFromArray([]*schema.Message{
		{Content: `{"code":"ignored"}`},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	facade := &NoCodeAIGenFacade{}
	reader, err := facade.processCodeStream(ctx, respStream, 1, enum.HtmlCodeGen)
	require.NoError(t, err)
	defer reader.Close()

	_, recvErr := reader.Recv()
	require.ErrorIs(t, recvErr, context.Canceled)
}
