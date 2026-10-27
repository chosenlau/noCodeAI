package service

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core/task"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/eino/schema"
)

type IAppService interface {
	ListSourceVersions(ctx context.Context, appID int64, user *api.UserVo) (api.SourceVersionsVo, error)
	RestoreSourceVersion(ctx context.Context, appID int64, req api.RestoreSourceRequest, user *api.UserVo) (int64, error)
	GraphToGenCode(ctx context.Context, appId int64, message string, loginUser *api.UserVo) (*schema.StreamReader[*schema.Message], *state.WorkFlowContext, error)
	StartGraphRun(ctx context.Context, req *api.GraphRunCreateRequest, loginUser *api.UserVo) (task.Run, bool, error)
	GetGraphRun(ctx context.Context, runID int64, loginUser *api.UserVo) (task.Run, error)
	ControlGraphRun(ctx context.Context, runID int64, action, hash string, loginUser *api.UserVo) (task.Run, error)
	GetActiveGraphRun(ctx context.Context, appID int64, loginUser *api.UserVo) (task.Run, error)
	SubscribeGraphRun(ctx context.Context, runID, afterSequence int64, loginUser *api.UserVo) (*task.Subscription, error)
	ChatWithAgent(ctx context.Context, appId int64, message string, loginUser *api.UserVo) (*schema.StreamReader[*schema.Message], error)
	GetSourceCode(ctx context.Context, appId int64, generationType string, loginUser *api.UserVo) (map[string]string, error)
	AddApp(ctx context.Context, req *api.NoCodeAppAddRequest, userId int64) (int64, error)
	UpdateApp(ctx context.Context, req *api.NoCodeAppUpdateRequest, userId int64) (bool, error)
	DeleteApp(ctx context.Context, id int64, userId int64) (bool, error)
	GetApp(ctx context.Context, id int64, userId int64) (*model.App, error)
	GetAppVo(ctx context.Context, id int64, userId int64) (api.AppVo, error)
	GetAppVoList(ctx context.Context, appList []*model.App) ([]api.AppVo, error)
	ListMyApp(ctx context.Context, req *api.NoCodeAppMyListRequest, userId int64) (*response.PageResponse[api.AppVo], error)
	ListGoodApp(ctx context.Context, req *api.NoCodeAppFeaturedListRequest) (*response.PageResponse[api.AppVo], error)
	AdminUpdateApp(ctx context.Context, req *api.NoCodeAppAdminUpdateRequest) (bool, error)
	AdminDeleteApp(ctx context.Context, id int64) (bool, error)
	AdminGetAppVo(ctx context.Context, id int64) (api.AppVo, error)
	AdminListApp(ctx context.Context, req *api.NoCodeAppAdminListRequest) (*response.PageResponse[*model.App], error)
}
