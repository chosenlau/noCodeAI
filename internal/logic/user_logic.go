package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/dal/query"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gen"
	"gorm.io/gen/field"
	"gorm.io/gorm"
)

type UserService struct {
	db          *gorm.DB
	redisClient *redis.Client
}

func NewUserService(db *gorm.DB, redisClient *redis.Client) *UserService {
	fmt.Printf("[DEBUG-wire-injection] NewUserService called: db=%p, redis=%p\n", db, redisClient)
	if db == nil {
		fmt.Println("[DEBUG-wire-injection] WARNING: db is nil in NewUserService!")
	}
	return &UserService{
		db:          db,
		redisClient: redisClient,
	}
}
func (s *UserService) HashPassword(ctx context.Context, password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func (s *UserService) CheckPassword(ctx context.Context, password, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

func (s *UserService) UserRegister(ctx context.Context, req *api.NoCodeRegisterRequest) (int64, error) {
	if req.UserAccount == "" || req.UserPassword == "" || req.CheckPassword == "" {
		return 0, errorutil.ParamsError
	}
	if len(req.UserAccount) < 4 || len(req.UserAccount) > 12 {
		return 0, errorutil.ParamsError.WithMessage("Username length must be between 4 and 12 characters.")
	}
	if len(req.UserPassword) < 8 || len(req.UserPassword) > 12 {
		return 0, errorutil.ParamsError.WithMessage("Password length must be between 8 and 12 characters.")
	}
	if req.UserPassword != req.CheckPassword {
		return 0, errorutil.ParamsError.WithMessage("The two password inputs do not match.")
	}
	u := query.Use(s.db)

	count, err := u.User.WithContext(ctx).Where(u.User.UserAccount.Eq(req.UserAccount)).Count()
	if err != nil && err != gorm.ErrRecordNotFound {
		return 0, errorutil.SystemError.WithMessage("Failed to check user account.")
	}
	if count > 0 {
		return 0, errorutil.ParamsError.WithMessage("Username already exists.")
	}

	hashedPassword, err := s.HashPassword(ctx, req.UserPassword)
	if err != nil {
		return 0, errorutil.SystemError.WithMessage("Failed to hash password.")
	}
	userID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return 0, errorutil.SystemError.WithMessage("Failed to generate user ID.")
	}

	newUser := &model.User{
		ID:           userID,
		UserAccount:  req.UserAccount,
		UserPassword: hashedPassword,
		UserName:     "default",
		UserRole:     string(enum.RoleUser),
	}
	err = u.User.WithContext(ctx).Create(newUser)
	if err != nil {
		return 0, errorutil.SystemError.WithMessage("Failed to create user")
	}
	return newUser.ID, nil
}

func (s *UserService) UserLogin(ctx context.Context, req *api.NoCodeLoginRequest) (*api.UserVo, string, error) {
	if req.UserAccount == "" || req.UserPassword == "" {
		return nil, "", errorutil.ParamsError
	}

	q := query.Use(s.db)
	user, err := q.User.WithContext(ctx).Where(q.User.UserAccount.Eq(req.UserAccount)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", errorutil.ParamsError
		}
		return nil, "", errorutil.SystemError.WithMessage("Failed to query user.")
	}
	if err := s.CheckPassword(ctx, req.UserPassword, user.UserPassword); err != nil {
		return nil, "", errorutil.ParamsError.WithMessage("Incorrect password.")
	}
	// 4. 生成 sessionId
	sessionId := fmt.Sprintf("session:%d", time.Now().UnixNano())

	userVo := api.UserVo{
		ID:          user.ID,
		UserAccount: user.UserAccount,
		UserName:    user.UserName,
		UserAvatar:  user.UserAvatar,
		UserProfile: user.UserProfile,
		UserRole:    user.UserRole,
		CreateTime:  user.CreateTime,
		UpdateTime:  user.UpdateTime,
	}
	// 关键步骤：将用户信息转换为json并存入Redis
	userVoJson, err := json.Marshal(userVo)
	if err != nil {
		return nil, "", err
	}
	err = s.redisClient.Set(ctx, sessionId, userVoJson, 24*time.Hour).Err()
	if err != nil {
		return nil, "", err
	}

	return &userVo, sessionId, nil
}

func (s *UserService) GetLoginUserVo(ctx context.Context, sessionId string) (*api.UserVo, error) {
	fmt.Printf("[DEBUG-wire-injection] GetLoginUserVo called: s=%p, s.db=%p\n", s, s.db)
	if s.db == nil {
		fmt.Println("[DEBUG-wire-injection] PANIC IMMINENT: s.db is nil!")
	}
	decodedSessionId, err := url.QueryUnescape(string(sessionId))
	if err != nil {
		return nil, err
	}
	// 关键步骤：从Redis获取用户信息
	userJson, err := s.redisClient.Get(ctx, decodedSessionId).Result()
	if err != nil {
		return nil, err
	}
	var userVo api.UserVo
	err = json.Unmarshal([]byte(userJson), &userVo)
	if err != nil {
		return nil, err
	}
	q := query.Use(s.db)
	_, err = q.User.WithContext(ctx).Where(q.User.ID.Eq(userVo.ID), q.User.IsDelete.Eq(0)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorutil.ParamsError.WithMessage("failed to verify user")
		}
		return nil, errorutil.SystemError.WithMessage("Failed to query user.")
	}
	userVo = api.UserVo{
		ID:          userVo.ID,
		UserAccount: userVo.UserAccount,
		UserName:    userVo.UserName,
		UserAvatar:  userVo.UserAvatar,
		UserProfile: userVo.UserProfile,
		UserRole:    userVo.UserRole,
		CreateTime:  userVo.CreateTime,
		UpdateTime:  userVo.UpdateTime,
	}
	return &userVo, nil
}

func (s *UserService) UserLogout(ctx context.Context, sessionId string, userId int64) error {
	if sessionId == "" {
		return nil
	}
	if err := s.redisClient.Del(ctx, sessionId).Err(); err != nil {
		return errorutil.SystemError.WithMessage("Failed to clear login session.")
	}
	return nil
}

func (s *UserService) GetUserByID(ctx context.Context, id int64) (*api.UserVo, error) {
	if id <= 0 {
		return nil, errorutil.ParamsError
	}
	q := query.Use(s.db)
	user, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id), q.User.IsDelete.Eq(0)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorutil.ParamsError
		}
		return nil, errorutil.SystemError.WithMessage("Failed to query user.")
	}
	userVo := api.UserVo{
		ID:          user.ID,
		UserAccount: user.UserAccount,
		UserName:    user.UserName,
		UserAvatar:  user.UserAvatar,
		UserProfile: user.UserProfile,
		UserRole:    user.UserRole,
		CreateTime:  user.CreateTime,
		UpdateTime:  user.UpdateTime,
	}
	return &userVo, nil
}

func (s *UserService) DeleteUser(ctx context.Context, req *api.NoCodeUserUpdateRequest) error {
	if req.Id <= 0 {
		return errorutil.ParamsError
	}
	u := query.Use(s.db).User
	info, err := u.WithContext(ctx).Where(u.ID.Eq(req.Id), u.IsDelete.Eq(0)).Update(u.IsDelete, 1)
	if err != nil {
		return errorutil.SystemError.WithMessage("Failed to delete user.")
	}
	if info.RowsAffected == 0 {
		return errorutil.ParamsError.WithMessage("User not found or already deleted.")
	}
	return nil
}

func (s *UserService) UpdateUser(ctx context.Context, req *api.NoCodeUserUpdateRequest) error {
	if req == nil || req.Id <= 0 {
		return errorutil.ParamsError.WithMessage("Invalid user ID.")
	}

	u := query.Use(s.db).User

	var updates []field.AssignExpr

	if req.UserName != "" {
		updates = append(updates, u.UserName.Value(req.UserName))
	}
	if req.UserAvatar != "" {
		updates = append(updates, u.UserAvatar.Value(req.UserAvatar))
	}
	if req.UserProfile != "" {
		updates = append(updates, u.UserProfile.Value(req.UserProfile))
	}
	if req.UserRole != "" {
		if !enum.IsValidRole(req.UserRole) {
			return errorutil.ParamsError.WithMessage("Invalid user role.")
		}
		updates = append(updates, u.UserRole.Value(req.UserRole))
	}

	if len(updates) == 0 {
		return nil
	}

	info, err := u.WithContext(ctx).Where(u.ID.Eq(req.Id), u.IsDelete.Eq(0)).UpdateSimple(updates...)
	if err != nil {
		return errorutil.SystemError.WithMessage("Failed to update user.")
	}
	if info.RowsAffected == 0 {
		return errorutil.ParamsError
	}

	return nil
}

func (s *UserService) AddUser(ctx context.Context, req *api.NoCodeUserAddRequest) (int64, error) {
	if req.UserAccount == "" || req.UserPassword == "" {
		return 0, errorutil.ParamsError
	}
	if !enum.IsValidRole(req.UserRole) {
		return 0, errorutil.ParamsError.WithMessage("Invalid user role.")
	}
	hashPassword, err := s.HashPassword(ctx, req.UserPassword)
	if err != nil {
		return 0, errorutil.SystemError.WithMessage("Failed to hash password.")
	}
	userID, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return 0, errorutil.SystemError.WithMessage("Failed to generate user ID.")
	}
	newUser := &model.User{
		ID:           userID,
		UserAccount:  req.UserAccount,
		UserPassword: hashPassword,
		UserAvatar:   req.UserAvatar,
		UserProfile:  req.UserProfile,
		UserRole:     req.UserRole,
	}
	u := query.Use(s.db).User
	err = u.WithContext(ctx).Create(newUser)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return 0, errorutil.ParamsError.WithMessage("User account already exists.")
		}
		return 0, errorutil.SystemError.WithMessage("Failed to add user.")
	}
	return userID, nil
}

func (s *UserService) ListUsersVoByPage(ctx context.Context, req *api.NoCodeUserQueryRequest) (*response.PageResponse[api.UserVo], error) {
	// 1. Set default pagination values (Remove the strict error return)
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}

	u := query.Use(s.db).User
	var conditions []gen.Condition

	// 2. Build query conditions
	conditions = append(conditions, u.IsDelete.Eq(0)) // Always filter soft-deleted users

	if req.UserAccount != "" {
		conditions = append(conditions, u.UserAccount.Eq(req.UserAccount))
	}
	if req.UserName != "" {
		conditions = append(conditions, u.UserName.Like("%"+req.UserName+"%"))
	}
	if req.UserRole != "" {
		if !enum.IsValidRole(req.UserRole) {
			return nil, errorutil.ParamsError.WithMessage("Invalid user role.")
		}
		conditions = append(conditions, u.UserRole.Eq(req.UserRole))
	}
	if req.UserProfile != "" {
		conditions = append(conditions, u.UserProfile.Like("%"+req.UserProfile+"%"))
	}

	// 3. Keep the query builder instance
	stmt := u.WithContext(ctx).Where(conditions...)

	// 4. Get the total count
	count, err := stmt.Count()
	if err != nil {
		return nil, errorutil.SystemError.WithMessage("Failed to count users.")
	}

	// If count is 0, return immediately to save a database query
	if count == 0 {
		return &response.PageResponse[api.UserVo]{
			TotalPage: 0,
			TotalRow:  0,
			PageSize:  int(req.PageSize),
			PageNum:   int(req.PageNum),
			Records:   make([]api.UserVo, 0),
		}, nil
	}

	// 5. Build safe order expression (White-list mapping)
	sortFieldMap := map[string]field.Expr{
		"id":          u.ID, // Fixed: u -> u.ID
		"create_time": u.CreateTime,
		"update_time": u.UpdateTime,
		"user_name":   u.UserName,
	}

	// Default sort by create_time descending
	orderExpr := u.CreateTime.Desc()

	if req.SortField != "" {
		targetField, exists := sortFieldMap[strings.ToLower(req.SortField)]
		if exists {
			// Apply ascending order only if explicitly specified, otherwise default to descending
			if strings.ToLower(req.SortOrder) == "asc" || strings.ToLower(req.SortOrder) == "ascend" {
				orderExpr = targetField.Asc()
			} else {
				orderExpr = targetField.Desc()
			}
		}
	}

	// 6. Query the paginated list
	// Note: You may need to cast PageNum/PageSize to int if they are int32/int64 in your request struct
	userList, err := stmt.Order(orderExpr).
		Offset(int((req.PageNum - 1) * req.PageSize)).
		Limit(int(req.PageSize)).
		Find()
	if err != nil {
		return nil, errorutil.SystemError.WithMessage("Failed to query users.")
	}

	// 7. Map Model to VO
	// Pre-allocate slice capacity for better performance
	userVoList := make([]api.UserVo, 0, len(userList))
	for _, user := range userList {
		userVoList = append(userVoList, api.UserVo{
			ID:          user.ID,
			UserAccount: user.UserAccount,
			UserName:    user.UserName,
			UserAvatar:  user.UserAvatar,
			UserProfile: user.UserProfile,
			UserRole:    user.UserRole,
			CreateTime:  user.CreateTime,
			UpdateTime:  user.UpdateTime,
		})
	}
	totalPage := int((count + int64(req.PageSize) - 1) / int64(req.PageSize))
	return &response.PageResponse[api.UserVo]{
		TotalPage: totalPage,
		TotalRow:  int(count),
		PageSize:  int(req.PageSize),
		PageNum:   int(req.PageNum),
		Records:   userVoList,
	}, nil
}
