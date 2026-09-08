package constants

type contextKey string

const (
	UserIDKey   contextKey = "current_user_id"
	UserRoleKey contextKey = "current_user_role"
)
