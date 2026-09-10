package api

import (
	"time"

	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/pkg/request"
	"github.com/chosenlau/noCodeAI/pkg/response"
)

type NoCodeAppAddResponse response.BaseResponse[string]

type NoCodeAppUpdateResponse response.BaseResponse[bool]

type NoCodeAppDeleteResponse response.BaseResponse[bool]

type NoCodeAppGetResponse response.BaseResponse[model.App]

type NoCodeAppGetVoResponse response.BaseResponse[AppVo]

type NoCodeAppMyListResponse response.BaseResponse[response.PageResponse[AppVo]]

type NoCodeAppFeaturedListResponse response.BaseResponse[response.PageResponse[AppVo]]

type NoCodeAppAdminUpdateResponse response.BaseResponse[bool]

type NoCodeAppAdminDeleteResponse response.BaseResponse[bool]

type NoCodeAppAdminGetResponse response.BaseResponse[AppVo]

type NoCodeAppAdminListResponse response.BaseResponse[response.PageResponse[model.App]]

type NoCodeAppAddRequest struct {
	InitPrompt string `json:"initPrompt"`
}

type NoCodeAppUpdateRequest struct {
	request.DeleteRequest
	AppName string `json:"appName"`
}

type NoCodeAppMyListRequest struct {
	request.PageRequest
	AppName string `json:"appName"`
}
type NoCodeAppFeaturedListRequest struct {
	request.PageRequest
	AppName     string `json:"appName"`
	CodeGenType string `json:"codeGenType"`
	InitPrompt  string `json:"initPrompt"`
	Priority    int32  `json:"priority"`
}

type NoCodeAppAdminUpdateRequest struct {
	Id       string `json:"id"`
	AppName  string `json:"appName"`
	Cover    string `json:"cover"`
	Priority int32  `json:"priority"`
}

type NoCodeAppAdminListRequest struct {
	request.PageRequest
	ID           string `json:"id"`
	AppName      string `json:"appName"`
	Cover        string `json:"cover"`
	InitPrompt   string `json:"initPrompt"`
	CodeGenType  string `json:"codeGenType"`
	DeployKey    string `json:"deployKey"`
	DeployedTime string `json:"deployedTime"`
	Priority     int32  `json:"priority"`
	UserID       int64  `json:"userId"`
}

type AppVo struct {
	ID           int64     `json:"id"`
	AppName      string    `json:"appName"`
	Cover        string    `json:"cover"`
	InitPrompt   string    `json:"initPrompt"`
	CodeGenType  string    `json:"codeGenType"`
	DeployKey    string    `json:"deployKey"`
	DeployedTime time.Time `json:"deployedTime"`
	Priority     int32     `json:"priority"`
	UserID       int64     `json:"userId"`
	User         UserVo    `json:"user"`
	CreateTime   time.Time `json:"createTime"`
	UpdateTime   time.Time `json:"updateTime"`
}
