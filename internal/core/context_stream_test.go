package core

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/chosenlau/noCodeAI/internal/core/execution"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestContextStatusBypassesCodeParserAndSaver(t *testing.T) {
	root := t.TempDir()
	ctx := execution.WithWorkspace(context.Background(), &execution.Workspace{AppID: 1, Root: root})
	status := contextmgr.EventMessage("context_compacting")
	// Even a future human-readable status must never enter generated JSON.
	status.Content = "正在整理任务信息"
	facade := &NoCodeAIGenFacade{codeSaver: &saver.CodeSaver{}}
	stream, err := facade.processCodeStream(ctx, schema.StreamReaderFromArray([]*schema.Message{
		status, schema.AssistantMessage(`{"htmlCode":"<h1>site</h1>","description":"done"}`, nil),
	}), 1, enum.HtmlCodeGen)
	require.NoError(t, err)
	defer stream.Close()
	first, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "context_compacting", contextmgr.EventKind(first))
	_, err = stream.Recv()
	require.NoError(t, err)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
	contents, err := os.ReadFile(filepath.Join(root, "index.html"))
	require.NoError(t, err)
	require.Equal(t, "<h1>site</h1>", string(contents))
}

func TestVueStreamPreservesStatusMetadata(t *testing.T) {
	status := contextmgr.EventMessage("context_ready")
	facade := &NoCodeAIGenFacade{}
	stream, err := facade.processVueCodeStream(context.Background(), schema.StreamReaderFromArray([]*schema.Message{status}))
	require.NoError(t, err)
	defer stream.Close()
	msg, err := stream.Recv()
	require.NoError(t, err)
	require.Same(t, status, msg)
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}
