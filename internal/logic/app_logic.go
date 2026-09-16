package logic

import (
	"context"
	"strconv"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/dal/query"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"github.com/cloudwego/eino/schema"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type AppService struct {
	aiCodeGenFacade    *core.NoCodeAIGenFacade // AI 代码生成门面
	userService        service.IUserService    // 用户服务接口
	chatHistoryService service.IChatHistoryService
	redisClient        *redis.Client
	db                 *gorm.DB // 数据库连接
}

func NewAppService(
	aiCodeGenFacade *core.NoCodeAIGenFacade,
	userService service.IUserService,
	chatHistoryService service.IChatHistoryService,
	db *gorm.DB,
	redisClient *redis.Client,
) *AppService {
	return &AppService{
		aiCodeGenFacade:    aiCodeGenFacade,
		userService:        userService,
		chatHistoryService: chatHistoryService,
		db:                 db,
		redisClient:        redisClient,
	}
}

func appMemoryKey(appID int64) string {
	return "memory:" + strconv.FormatInt(appID, 10)
}

func (s *AppService) AddApp(ctx context.Context, req *api.NoCodeAppAddRequest, userId int64) (int64, error) {
	// 1. 参数校验
	if req.InitPrompt == "" {
		return 0, errorutil.ParamsError.WithMessage("初始化prompt不能为空")
	}

	// 2. 生成应用名称（截取前12个字符）
	appName := req.InitPrompt
	count := 0
	for i := range appName {
		if count >= 12 {
			appName = appName[:i]
			break
		}
		count++
	}

	// 3. 生成应用 ID（雪花算法）
	appId, err := snowflake.GenerateSnowFlakeId()
	if err != nil {
		return 0, err
	}

	// 4. 构建应用实体
	newApp := &model.App{
		ID:          appId,
		AppName:     appName,
		InitPrompt:  req.InitPrompt,
		UserID:      userId,
		CodeGenType: string(enum.HtmlCodeGen),
		Priority:    0,
	}

	// 5. 保存到数据库
	q := query.Use(s.db)
	err = q.App.WithContext(ctx).
		Select(q.App.ID, q.App.AppName, q.App.InitPrompt,
			q.App.UserID, q.App.Priority, q.App.CodeGenType).
		Create(newApp)
	if err != nil {
		return 0, err
	}

	logger.Infof("应用创建成功，ID: %d, 类型: %s", appId, enum.HtmlCodeGen)
	return newApp.ID, nil
}

func (s *AppService) UpdateApp(ctx context.Context, req *api.NoCodeAppUpdateRequest, userId int64) (bool, error) {
	// 1. 参数校验
	if req.Id == 0 {
		return false, errorutil.ParamsError.WithMessage("应用ID不能为空")
	}

	q := query.Use(s.db)
	// 2. 查询应用
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(int64(req.Id)), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return false, err
	}

	// 3. 权限校验
	if app.UserID != userId {
		return false, errorutil.ParamsError.WithMessage("无权修改该应用")
	}

	// 4. 构建更新字段
	updateMap := make(map[string]interface{})
	if req.AppName != "" {
		updateMap["appName"] = req.AppName
	}

	// 5. 执行更新
	_, err = q.App.WithContext(ctx).Where(q.App.ID.Eq(int64(req.Id)), q.App.IsDelete.Eq(0)).Updates(updateMap)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *AppService) DeleteApp(ctx context.Context, id int64, userId int64) (bool, error) {
	q := query.Use(s.db)
	err := q.Transaction(func(tx *query.Query) error {
		app, err := tx.App.WithContext(ctx).Where(tx.App.ID.Eq(id), tx.App.IsDelete.Eq(0)).First()
		if err != nil {
			return err
		}
		if app.UserID != userId {
			return errorutil.ParamsError.WithMessage("无权删除该应用")
		}
		info, err := tx.App.WithContext(ctx).
			Where(tx.App.ID.Eq(id), tx.App.IsDelete.Eq(0)).
			Update(tx.App.IsDelete, 1)
		if err != nil {
			return errorutil.SystemError.WithMessage("failed to delete app")
		}
		if info.RowsAffected == 0 {
			return errorutil.ParamsError.WithMessage("app not found or already deleted")
		}
		_, err = tx.ChatHistory.WithContext(ctx).
			Where(tx.ChatHistory.AppID.Eq(id), tx.ChatHistory.IsDelete.Eq(0)).
			Update(tx.ChatHistory.IsDelete, 1)
		if err != nil {
			return errorutil.SystemError.WithMessage("failed to delete chat history")
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if s.redisClient != nil {
		if err := s.redisClient.Del(ctx, appMemoryKey(id)).Err(); err != nil {
			return false, errorutil.SystemError.WithMessage("failed to clear app memory")
		}
	}
	return true, nil
}

func (s *AppService) GetAppVo(ctx context.Context, id int64, userId int64) (api.AppVo, error) {
	// 1. 获取应用实体
	app, err := s.GetApp(ctx, id, userId)
	if err != nil {
		return api.AppVo{}, err
	}

	// 2. 获取用户信息
	userVo, err := s.userService.GetUserByID(ctx, app.UserID)
	if err != nil {
		return api.AppVo{}, err
	}

	// 3. 构建应用 VO
	appVo := api.AppVo{
		ID:           app.ID,
		AppName:      app.AppName,
		Cover:        app.Cover,
		InitPrompt:   app.InitPrompt,
		CodeGenType:  app.CodeGenType,
		DeployKey:    app.DeployKey,
		DeployedTime: app.DeployedTime,
		Priority:     app.Priority,
		UserID:       app.UserID,
		User:         *userVo,
		CreateTime:   app.CreateTime,
		UpdateTime:   app.UpdateTime,
	}
	return appVo, nil
}

func (s *AppService) GetApp(ctx context.Context, id int64, userId int64) (*model.App, error) {
	q := query.Use(s.db)
	// 1. 查询应用
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(id), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return nil, err
	}

	// 2. 权限校验
	if app.UserID != userId {
		return nil, errorutil.ParamsError.WithMessage("无权查看该应用")
	}
	return app, nil
}

func (s *AppService) ListMyApp(ctx context.Context, req *api.NoCodeAppMyListRequest, userId int64) (*response.PageResponse[api.AppVo], error) {
	// 1. 参数校验和默认值设置
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 20 {
		req.PageSize = 20
	}

	q := query.Use(s.db)
	// 2. 构建查询条件
	queryBuilder := q.App.WithContext(ctx).Where(q.App.IsDelete.Eq(0), q.App.UserID.Eq(userId))

	if req.AppName != "" {
		queryBuilder = queryBuilder.Where(q.App.AppName.Like("%" + req.AppName + "%"))
	}

	// 3. 查询总数
	totalCount, err := queryBuilder.Count()
	if err != nil {
		return nil, err
	}

	// 4. 计算分页信息
	totalPage := int((totalCount + int64(req.PageSize) - 1) / int64(req.PageSize))
	offset := (req.PageNum - 1) * req.PageSize

	// 5. 设置排序
	if req.SortField != "" {
		if orderExpr, ok := q.App.GetFieldByName(req.SortField); ok {
			if req.SortOrder == "desc" {
				queryBuilder = queryBuilder.Order(orderExpr.Desc())
			} else {
				queryBuilder = queryBuilder.Order(orderExpr)
			}
		} else {
			queryBuilder = queryBuilder.Order(q.App.CreateTime.Desc())
		}
	} else {
		queryBuilder = queryBuilder.Order(q.App.CreateTime.Desc())
	}

	// 6. 执行分页查询
	appList, err := queryBuilder.Offset(offset).Limit(req.PageSize).Find()
	if err != nil {
		return nil, err
	}

	// 7. 转换为AppVo列表
	appVoList, err := s.GetAppVoList(ctx, appList)
	if err != nil {
		return nil, err
	}

	// 8. 构建分页响应
	pageResponse := &response.PageResponse[api.AppVo]{
		Records:            appVoList,
		PageNum:            req.PageNum,
		PageSize:           req.PageSize,
		TotalPage:          totalPage,
		TotalRow:           int(totalCount),
		OptimizeCountQuery: false,
	}

	return pageResponse, nil
}

func (s *AppService) GetAppVoList(ctx context.Context, appList []*model.App) ([]api.AppVo, error) {
	// 批量获取用户信息（去重）
	userIdSet := make(map[int64]bool)
	for _, app := range appList {
		userIdSet[app.UserID] = true
	}

	// 转换为切片
	userIdList := make([]int64, 0, len(userIdSet))
	for userId := range userIdSet {
		userIdList = append(userIdList, userId)
	}

	// 获取所有用户信息
	q := query.Use(s.db)
	userList, err := q.WithContext(ctx).User.Where(q.User.ID.In(userIdList...)).Find()
	if err != nil {
		return nil, err
	}
	userVoMap := make(map[int64]api.UserVo)
	for _, dbUser := range userList {
		userVoMap[dbUser.ID] = api.UserVo{
			ID:          dbUser.ID,
			UserAccount: dbUser.UserAccount,
			UserName:    dbUser.UserName,
			UserAvatar:  dbUser.UserAvatar,
			UserProfile: dbUser.UserProfile,
			UserRole:    dbUser.UserRole,
			CreateTime:  dbUser.CreateTime,
			UpdateTime:  dbUser.UpdateTime,
		}
	}

	// 转换为AppVo列表
	var appVoList []api.AppVo
	for _, app := range appList {
		appVo := api.AppVo{
			ID:           app.ID,
			AppName:      app.AppName,
			Cover:        app.Cover,
			InitPrompt:   app.InitPrompt,
			CodeGenType:  app.CodeGenType,
			DeployKey:    app.DeployKey,
			DeployedTime: app.DeployedTime,
			Priority:     app.Priority,
			UserID:       app.UserID,
			User:         userVoMap[app.UserID],
			CreateTime:   app.CreateTime,
			UpdateTime:   app.UpdateTime,
		}
		appVoList = append(appVoList, appVo)
	}

	return appVoList, nil
}

func (s *AppService) ListGoodApp(ctx context.Context, req *api.NoCodeAppFeaturedListRequest) (*response.PageResponse[api.AppVo], error) {
	// 1. 参数校验和默认值设置
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 20 {
		req.PageSize = 20
	}

	q := query.Use(s.db)
	// 2. 构建查询条件（精选应用：priority > 0）
	queryBuilder := q.App.WithContext(ctx).Where(q.App.IsDelete.Eq(0), q.App.Priority.Gt(0))

	// 3. 添加查询条件
	if req.AppName != "" {
		queryBuilder = queryBuilder.Where(q.App.AppName.Like("%" + req.AppName + "%"))
	}
	if req.CodeGenType != "" {
		queryBuilder = queryBuilder.Where(q.App.CodeGenType.Eq(req.CodeGenType))
	}
	if req.InitPrompt != "" {
		queryBuilder = queryBuilder.Where(q.App.InitPrompt.Like("%" + req.InitPrompt + "%"))
	}
	if req.Priority != 0 {
		queryBuilder = queryBuilder.Where(q.App.Priority.Eq(req.Priority))
	}

	// 4. 查询总数
	totalCount, err := queryBuilder.Count()
	if err != nil {
		return nil, err
	}

	// 5. 计算分页信息
	totalPage := int((totalCount + int64(req.PageSize) - 1) / int64(req.PageSize))
	offset := (req.PageNum - 1) * req.PageSize

	// 6. 设置排序（默认按优先级降序、创建时间降序）
	if req.SortField != "" {
		if orderExpr, ok := q.App.GetFieldByName(req.SortField); ok {
			if req.SortOrder == "desc" {
				queryBuilder = queryBuilder.Order(orderExpr.Desc())
			} else {
				queryBuilder = queryBuilder.Order(orderExpr)
			}
		} else {
			queryBuilder = queryBuilder.Order(q.App.Priority.Desc(), q.App.CreateTime.Desc())
		}
	} else {
		queryBuilder = queryBuilder.Order(q.App.Priority.Desc(), q.App.CreateTime.Desc())
	}

	// 7. 执行分页查询
	appList, err := queryBuilder.Offset(offset).Limit(req.PageSize).Find()
	if err != nil {
		return nil, err
	}

	// 8. 转换为AppVo列表
	appVoList, err := s.GetAppVoList(ctx, appList)
	if err != nil {
		return nil, err
	}

	// 9. 构建分页响应
	pageResponse := &response.PageResponse[api.AppVo]{
		Records:   appVoList,
		PageNum:   req.PageNum,
		PageSize:  req.PageSize,
		TotalPage: totalPage,
		TotalRow:  int(totalCount),
	}

	return pageResponse, nil
}

func (s *AppService) AdminUpdateApp(ctx context.Context, req *api.NoCodeAppAdminUpdateRequest) (bool, error) {
	// 1. 参数校验
	if req.Id == "" {
		return false, errorutil.ParamsError.WithMessage("应用ID不能为空")
	}
	appId, err := strconv.Atoi(req.Id)
	if err != nil {
		return false, err
	}

	q := query.Use(s.db)
	// 2. 查询应用
	_, err = q.App.WithContext(ctx).Where(q.App.ID.Eq(int64(appId)), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return false, err
	}

	// 3. 构建更新字段
	updateMap := make(map[string]interface{})
	if req.AppName != "" {
		updateMap["appName"] = req.AppName
	}
	if req.Cover != "" {
		updateMap["cover"] = req.Cover
	}
	updateMap["priority"] = req.Priority

	// 4. 执行更新
	_, err = q.App.WithContext(ctx).Where(q.App.ID.Eq(int64(appId)), q.App.IsDelete.Eq(0)).Updates(updateMap)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *AppService) AdminDeleteApp(ctx context.Context, id int64) (bool, error) {
	q := query.Use(s.db)
	err := q.Transaction(func(tx *query.Query) error {
		info, err := tx.App.WithContext(ctx).
			Where(tx.App.ID.Eq(id), tx.App.IsDelete.Eq(0)).
			Update(tx.App.IsDelete, 1)
		if err != nil {
			return err
		}
		if info.RowsAffected == 0 {
			return errorutil.ParamsError.WithMessage("app not found or already deleted")
		}
		_, err = tx.ChatHistory.WithContext(ctx).
			Where(tx.ChatHistory.AppID.Eq(id), tx.ChatHistory.IsDelete.Eq(0)).
			Update(tx.ChatHistory.IsDelete, 1)
		return err
	})
	if err != nil {
		return false, err
	}
	if s.redisClient != nil {
		if err := s.redisClient.Del(ctx, appMemoryKey(id)).Err(); err != nil {
			return false, errorutil.SystemError.WithMessage("failed to clear app memory")
		}
	}
	return true, nil
}

func (s *AppService) AdminGetAppVo(ctx context.Context, id int64) (api.AppVo, error) {
	q := query.Use(s.db)
	// 1. 查询应用
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(id), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return api.AppVo{}, err
	}

	// 2. 获取用户信息
	userVo, err := s.userService.GetUserByID(ctx, app.UserID)
	if err != nil {
		return api.AppVo{}, err
	}

	// 3. 构建应用 VO
	appVo := api.AppVo{
		ID:           app.ID,
		AppName:      app.AppName,
		Cover:        app.Cover,
		InitPrompt:   app.InitPrompt,
		CodeGenType:  app.CodeGenType,
		DeployKey:    app.DeployKey,
		DeployedTime: app.DeployedTime,
		Priority:     app.Priority,
		UserID:       app.UserID,
		User:         *userVo,
		CreateTime:   app.CreateTime,
		UpdateTime:   app.UpdateTime,
	}
	return appVo, nil
}

func (s *AppService) AdminListApp(ctx context.Context, req *api.NoCodeAppAdminListRequest) (*response.PageResponse[*model.App], error) {
	// 1. 参数校验和默认值设置
	if req.PageNum <= 0 {
		req.PageNum = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 20 {
		req.PageSize = 20
	}

	q := query.Use(s.db)
	// 2. 构建查询条件
	queryBuilder := q.App.WithContext(ctx).Where(q.App.IsDelete.Eq(0))

	// 3. 添加查询条件
	if req.ID != "" {
		id, _ := strconv.ParseInt(req.ID, 10, 64)
		queryBuilder = queryBuilder.Where(q.App.ID.Eq(id))
	}
	if req.AppName != "" {
		queryBuilder = queryBuilder.Where(q.App.AppName.Like("%" + req.AppName + "%"))
	}
	if req.Cover != "" {
		queryBuilder = queryBuilder.Where(q.App.Cover.Like("%" + req.Cover + "%"))
	}
	if req.InitPrompt != "" {
		queryBuilder = queryBuilder.Where(q.App.InitPrompt.Like("%" + req.InitPrompt + "%"))
	}
	if req.CodeGenType != "" {
		queryBuilder = queryBuilder.Where(q.App.CodeGenType.Eq(req.CodeGenType))
	}
	if req.DeployKey != "" {
		queryBuilder = queryBuilder.Where(q.App.DeployKey.Like("%" + req.DeployKey + "%"))
	}
	if req.Priority != 0 {
		queryBuilder = queryBuilder.Where(q.App.Priority.Eq(req.Priority))
	}
	if req.UserID != 0 {
		queryBuilder = queryBuilder.Where(q.App.UserID.Eq(req.UserID))
	}

	// 4. 查询总数
	totalCount, err := queryBuilder.Count()
	if err != nil {
		return nil, err
	}

	// 5. 计算分页信息
	totalPage := int((totalCount + int64(req.PageSize) - 1) / int64(req.PageSize))
	offset := (req.PageNum - 1) * req.PageSize

	// 6. 设置排序
	if req.SortField != "" {
		if orderExpr, ok := q.App.GetFieldByName(req.SortField); ok {
			if req.SortOrder == "desc" {
				queryBuilder = queryBuilder.Order(orderExpr.Desc())
			} else {
				queryBuilder = queryBuilder.Order(orderExpr)
			}
		} else {
			queryBuilder = queryBuilder.Order(q.App.CreateTime.Desc())
		}
	} else {
		queryBuilder = queryBuilder.Order(q.App.CreateTime.Desc())
	}

	// 7. 执行分页查询
	appList, err := queryBuilder.Offset(offset).Limit(req.PageSize).Find()
	if err != nil {
		return nil, err
	}

	// 8. 构建分页响应
	pageResponse := &response.PageResponse[*model.App]{
		Records:   appList,
		PageNum:   req.PageNum,
		PageSize:  req.PageSize,
		TotalPage: totalPage,
		TotalRow:  int(totalCount),
	}

	return pageResponse, nil
}

func (s *AppService) ChatToGenCode(ctx context.Context, appId int64, message string, loginUser *api.UserVo) (*schema.StreamReader[*schema.Message], error) {
	// 1. 校验参数
	if message == "" {
		return nil, errorutil.ParamsError.WithMessage("消息不能为空")
	}
	if appId == 0 || appId < 0 {
		return nil, errorutil.ParamsError.WithMessage("应用ID不能为空")
	}

	q := query.Use(s.db)
	// 2. 校验应用是否存在
	app, err := q.App.WithContext(ctx).Where(q.App.ID.Eq(appId), q.App.IsDelete.Eq(0)).First()
	if err != nil {
		return nil, err
	}

	// 3. 校验用户是否有权限使用该应用
	if app.UserID != loginUser.ID {
		return nil, errorutil.NotAuthError.WithMessage("无权使用该应用")
	}

	// 4. 获取代码生成类型
	if enum.CodeGenTypeTextMap[enum.CodeGenTypeEnum(app.CodeGenType)] == "" {
		return nil, errorutil.ParamsError.WithMessage("应用代码生成类型不支持")
	}

	err = s.chatHistoryService.AddChatMessage(ctx, appId, message, enum.UserMessageType, loginUser.ID)
	if err != nil {
		return nil, errorutil.SystemError.WithMessage("failed to save chat history")
	}
	// 5. 调用代码生成服务
	return s.aiCodeGenFacade.GenCodeStreamAndSave(ctx, appId, message, enum.CodeGenTypeEnum(app.CodeGenType))
}
