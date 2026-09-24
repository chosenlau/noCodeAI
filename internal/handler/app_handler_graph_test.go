package handler

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/test/mock"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/stretchr/testify/require"
)

type fakeGraphAppService struct {
	service.IAppService
}

func (s *fakeGraphAppService) GetSourceCode(context.Context, int64, string, *api.UserVo) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *fakeGraphAppService) GraphToGenCode(context.Context, int64, string, *api.UserVo) (*schema.StreamReader[*schema.Message], *state.WorkFlowContext, error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	workflowContext := &state.WorkFlowContext{
		QualityResult: aimodel.QualityResult{IsValid: true},
		CodeContent: map[string]string{
			"src/App.vue": "<template>fake</template>",
			"src/main.js": "console.log('fake')",
		},
	}
	go func() {
		defer writer.Close()
		_ = writer.Send(&schema.Message{Content: `{"stepNumber":1,"currentStep":"code generation"}`}, nil)
		_ = writer.Send(&schema.Message{Content: `{"stepNumber":2,"currentStep":"quality check"}`}, nil)
	}()
	return reader, workflowContext, nil
}

func TestAppHandlerGraphToGenCodeFake(t *testing.T) {
	handler := NewAppHandler(&fakeGraphAppService{}, nil, nil)
	ctx := app.NewContext(4)
	ctx.Request.SetRequestURI("/app/graph")
	ctx.Request.Header.SetMethod(consts.MethodPost)
	ctx.Request.Header.SetContentTypeBytes([]byte("application/json"))
	requestBody := `{"appId":"1","message":"create homepage"}`
	ctx.Request.SetBodyString(requestBody)
	ctx.Request.Header.SetContentLength(len(requestBody))
	ctx.Response.SetBodyString("")
	conn := mock.NewConn("")
	ctx.SetConn(conn)
	ctx.Set(constants.UserVoKey, &api.UserVo{ID: 7})

	handler.GraphToGenCode(context.Background(), ctx)

	bodyBytes, _ := conn.WriterRecorder().Peek(conn.WriterRecorder().WroteLen())
	body := string(bodyBytes)
	require.Contains(t, body, "event: step_completed")
	require.Contains(t, body, "code generation")
	require.Contains(t, body, "quality check")
	require.Contains(t, body, "event: code_completed")
	require.Contains(t, body, `"src/App.vue":"\u003ctemplate\u003efake\u003c/template\u003e"`)
	require.Contains(t, body, "event: done")
}

func TestAppHandlerGraphToGenCodeRealLLM(t *testing.T) {
	if os.Getenv("RUN_REAL_LLM") != "1" {
		t.Skip("set RUN_REAL_LLM=1 to run the real LLM handler test")
	}

	t.Skip("real handler wiring requires an application fixture and configured database")
}

func TestGraphCodeContentJSONContract(t *testing.T) {
	content := map[string]string{"src/App.vue": "<template />"}
	data, err := json.Marshal(content)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(data), `{"src/App.vue"`))
}

var _ service.IAppService = (*fakeGraphAppService)(nil)
