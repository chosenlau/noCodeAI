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

type NoCodeGenCodeRequest struct {
	AppId   int64  `json:"appId,string" vd:"$>0"` // 加 string 标签防止大整数精度丢失，vd 标签用于参数校验
	Message string `json:"message" vd:"len($)>0"`
}

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
	ID               int64     `json:"id,string"`
	AppName          string    `json:"appName"`
	Cover            string    `json:"cover"`
	InitPrompt       string    `json:"initPrompt"`
	CodeGenType      string    `json:"codeGenType"`
	DeployKey        string    `json:"deployKey"`
	DeployedTime     time.Time `json:"deployedTime"`
	Priority         int32     `json:"priority"`
	UserID           int64     `json:"userId,string"`
	User             UserVo    `json:"user"`
	CreateTime       time.Time `json:"createTime"`
	UpdateTime       time.Time `json:"updateTime"`
	TokenUsage       int64     `json:"tokenUsage"`
	PromptTokens     int64     `json:"promptTokens"`
	CompletionTokens int64     `json:"completionTokens"`
	Memory           MemoryVo  `json:"memory"`
}

type MemoryVo struct {
	Summary          string    `json:"summary"`
	Round            int64     `json:"round"`
	PromptTokens     int64     `json:"promptTokens"`
	CompletionTokens int64     `json:"completionTokens"`
	TotalTokens      int64     `json:"totalTokens"`
	Summarizing      bool      `json:"summarizing"`
	SummaryError     string    `json:"summaryError"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
