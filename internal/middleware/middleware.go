package middleware

import (
	"context"
	"strconv"

	"github.com/chosenlau/noCodeAI/pkg/constants"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func AuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		cookieUserID := c.Cookie("user_id")
		cookieUserRole := c.Cookie("user_role")

		if len(cookieUserID) == 0 || len(cookieUserRole) == 0 {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError.WithMessage("Login status not detected.")))
			c.Abort()
			return
		}

		userID, err := strconv.ParseInt(string(cookieUserID), 10, 64)
		if err != nil || userID <= 0 {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotLoginError.WithMessage("Invalid login credentials.")))
			c.Abort()
			return
		}

		ctx = context.WithValue(ctx, constants.UserIDKey, userID)
		ctx = context.WithValue(ctx, constants.UserRoleKey, string(cookieUserRole))
		c.Next(ctx)
	}
}

func DevRequireAdmin() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		role, ok := ctx.Value(constants.UserRoleKey).(string)
		if !ok || role != "admin" {
			c.JSON(consts.StatusOK, response.NewErrorResponse[any](errorutil.NotAuthError.WithMessage("Admin privileges required.")))
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}
