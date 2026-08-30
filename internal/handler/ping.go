package handler

import (
	"context"

	response "github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
 	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type PingResponse response.BaseResponse[string]

func Ping(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, response.NewSuccessResponse("pong"))
}
