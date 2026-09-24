package service

import (
	"context"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/pkg/response"
)

type IUserService interface {
	// UserRegister 创建用户
	UserRegister(ctx context.Context, req *api.NoCodeRegisterRequest) (int64, error)

	// UserLogin 用户登录
	UserLogin(ctx context.Context, req *api.NoCodeLoginRequest) (*api.UserVo, string, error)

	// GetLoginUserVo 根据 sessionId 从登录态中获取当前用户
	GetLoginUserVo(ctx context.Context, sessionId string) (*api.UserVo, error)

	// UserLogout 用户退出登录，删除 Redis session（并可选从用户 session 索引中移除）
	UserLogout(ctx context.Context, sessionId string, userId int64) error

	HashPassword(ctx context.Context, password string) (string, error)

	CheckPassword(ctx context.Context, password, hashedPassword string) error

	AddUser(ctx context.Context, req *api.NoCodeUserAddRequest) (int64, error)

	GetUserByID(ctx context.Context, id int64) (*api.UserVo, error)

	DeleteUser(ctx context.Context, req *api.NoCodeUserUpdateRequest) error

	UpdateUser(ctx context.Context, req *api.NoCodeUserUpdateRequest) error

	ListUsersVoByPage(ctx context.Context, req *api.NoCodeUserQueryRequest) (*response.PageResponse[api.UserVo], error)
}
