package api

import (
	"time"

	"github.com/chosenlau/noCodeAI/pkg/request"
	"github.com/chosenlau/noCodeAI/pkg/response"
)

type NoCodeRegisterResponse response.BaseResponse[int64]
type NoCodeLoginResponse response.BaseResponse[string]
type NocodeUserAddResponse response.BaseResponse[int64]
type NoCodeUserUpdateResponse response.BaseResponse[bool]
type NoCodeUserDeleteResponse response.BaseResponse[bool]
type NoCodeUserGetVoResponse response.BaseResponse[*UserVo]
type NoCodeUserPageVoResponse response.BaseResponse[*response.PageResponse[*UserVo]]

type NoCodeRegisterRequest struct {
	UserAccount   string `json:"userAccount"`
	UserPassword  string `json:"userPassword"`
	CheckPassword string `json:"checkPassword"`
}

type NoCodeLoginRequest struct {
	UserAccount  string `json:"userAccount"`
	UserPassword string `json:"userPassword"`
}

type UserVo struct {
	ID          int64     `json:"id,string"`
	UserAccount string    `json:"userAccount"`
	UserName    string    `json:"userName"`
	UserAvatar  string    `json:"userAvatar"`
	UserProfile string    `json:"userProfile"`
	UserRole    string    `json:"userRole"`
	CreateTime  time.Time `json:"createTime"`
	UpdateTime  time.Time `json:"updateTime"`
}

type NoCodeUserAddRequest struct {
	UserAccount  string `json:"userAccount"`
	UserPassword string `json:"userPassword"`
	UserAvatar   string `json:"userAvatar"`
	UserProfile  string `json:"userProfile"`
	UserRole     string `json:"userRole"`
}

type NoCodeUserUpdateRequest struct {
	request.DeleteRequest
	UserName    string `json:"userName"`
	UserAvatar  string `json:"userAvatar"`
	UserProfile string `json:"userProfile"`
	UserRole    string `json:"userRole"`
}

type NoCodeUserQueryRequest struct {
	request.PageRequest
	UserAccount string `json:"userAccount"`
	UserProfile string `json:"userProfile"`
	UserName    string `json:"userName"`
	UserRole    string `json:"userRole"`
}
