package service

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/eino/schema"
)

type IAppService interface {
	ChatToGenCode(ctx context.Context, appId int64, message string, loginUser *api.UserVo) (*schema.StreamReader[*schema.Message], error)
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
