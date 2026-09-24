package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/core/saver"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestProcessCodeStreamSavesAfterDownstreamReaderCloses(t *testing.T) {
	codeSaver, err := saver.NewCodeSaver()
	require.NoError(t, err)

	appID := time.Now().UnixNano()
	response, err := json.Marshal(aimodel.HtmlCodeResponse{
		HtmlCode:    "<main>saved</main>",
		Description: "saved description",
	})
	require.NoError(t, err)

	respStream := schema.StreamReaderFromArray([]*schema.Message{
		{Content: string(response)},
	})
	reader, err := (&NoCodeAIGenFacade{codeSaver: codeSaver}).processCodeStream(
		context.Background(),
		respStream,
		appID,
		enum.HtmlCodeGen,
	)
	require.NoError(t, err)
	reader.Close()

	dir, err := codeSaver.SavedDirPath(enum.HtmlCodeGen, appID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	require.Eventually(t, func() bool {
		_, err := os.Stat(dir + string(os.PathSeparator) + "index.html")
		return err == nil
	}, time.Second, 10*time.Millisecond)
}
