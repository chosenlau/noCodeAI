package enum

type UserRole string
type LoginState string

const (
	UserLoginState  LoginState = "user_login"
	AdminLoginState LoginState = "admin_login"
)
const (
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
)

var roleTextMap = map[UserRole]string{
	RoleAdmin: "admin",
	RoleUser:  "user",
}

func (r UserRole) GetRoleText() string {
	if text, exists := roleTextMap[r]; exists {
		return text
	}
	return "unknown"
}

func IsValidRole(role string) bool {
	switch UserRole(role) {
	case RoleAdmin, RoleUser:
		return true
	default:
		return false
	}
}
