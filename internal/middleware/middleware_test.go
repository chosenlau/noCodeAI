package middleware

import (
	"context"
	"testing"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/cloudwego/hertz/pkg/app"
)

type stubUserService struct {
	service.IUserService
	getLoginUserVo func(ctx context.Context, sessionId string) (*api.UserVo, error)
}

func (s stubUserService) GetLoginUserVo(ctx context.Context, sessionId string) (*api.UserVo, error) {
	return s.getLoginUserVo(ctx, sessionId)
}

func TestAuthMiddleware_MissingCookie_Aborts(t *testing.T) {
	ctx := app.NewContext(0)

	called := false
	userSvc := stubUserService{
		getLoginUserVo: func(ctx context.Context, sessionId string) (*api.UserVo, error) {
			t.Fatalf("GetLoginUserVo should not be called when cookie is missing")
			return nil, nil
		},
	}

	ctx.SetHandlers(app.HandlersChain{
		AuthMiddleware(userSvc),
		func(ctx context.Context, c *app.RequestContext) { called = true },
	})
	ctx.Handlers()[0](context.Background(), ctx)

	if !ctx.IsAborted() {
		t.Fatalf("expected aborted request")
	}
	if called {
		t.Fatalf("expected next handler not called")
	}
}

func TestAuthMiddleware_ValidCookie_PassesAndInjectsUser(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Request.Header.SetCookie(constants.UserLoginState, "good")

	called := false
	userSvc := stubUserService{
		getLoginUserVo: func(ctx context.Context, sessionId string) (*api.UserVo, error) {
			if sessionId != "good" {
				t.Fatalf("unexpected sessionId: %q", sessionId)
			}
			return &api.UserVo{ID: 1, UserRole: enum.RoleUser.GetRoleText()}, nil
		},
	}

	ctx.SetHandlers(app.HandlersChain{
		AuthMiddleware(userSvc),
		func(ctx context.Context, c *app.RequestContext) {
			called = true
			v, ok := c.Get(constants.UserVoKey)
			if !ok {
				t.Fatalf("expected user injected into context")
			}
			if _, ok := v.(*api.UserVo); !ok {
				t.Fatalf("expected *api.UserVo in context, got %T", v)
			}
		},
	})
	ctx.Handlers()[0](context.Background(), ctx)

	if ctx.IsAborted() {
		t.Fatalf("did not expect aborted request")
	}
	if !called {
		t.Fatalf("expected next handler called")
	}
}

func TestDevRequireAdmin_UserRoleDenied_Aborts(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Set(constants.UserVoKey, &api.UserVo{ID: 1, UserRole: enum.RoleUser.GetRoleText()})

	called := false
	ctx.SetHandlers(app.HandlersChain{
		DevRequireAdmin(),
		func(ctx context.Context, c *app.RequestContext) { called = true },
	})
	ctx.Handlers()[0](context.Background(), ctx)

	if !ctx.IsAborted() {
		t.Fatalf("expected aborted request")
	}
	if called {
		t.Fatalf("expected next handler not called")
	}
}

func TestDevRequireAdmin_AdminRoleAllowed_Passes(t *testing.T) {
	ctx := app.NewContext(0)
	ctx.Set(constants.UserVoKey, &api.UserVo{ID: 1, UserRole: enum.RoleAdmin.GetRoleText()})

	called := false
	ctx.SetHandlers(app.HandlersChain{
		DevRequireAdmin(),
		func(ctx context.Context, c *app.RequestContext) { called = true },
	})
	ctx.Handlers()[0](context.Background(), ctx)

	if ctx.IsAborted() {
		t.Fatalf("did not expect aborted request")
	}
	if !called {
		t.Fatalf("expected next handler called")
	}
}

