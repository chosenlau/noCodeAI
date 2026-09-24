package logic

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/ai/agent"
	graphnode "github.com/chosenlau/noCodeAI/internal/ai/graph/node"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/state"
	"github.com/chosenlau/noCodeAI/internal/ai/graph/workflow"
	"github.com/chosenlau/noCodeAI/internal/api"
	"github.com/chosenlau/noCodeAI/internal/core"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/dal/model"
	"github.com/chosenlau/noCodeAI/internal/dal/query"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/errorutil"
	file "github.com/chosenlau/noCodeAI/pkg/myfile"
	"github.com/chosenlau/noCodeAI/pkg/response"
	"github.com/chosenlau/noCodeAI/pkg/snowflake"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

type AppService struct {
	aiCodeGenFacade    *core.NoCodeAIGenFacade // AI 代码生成门面
	userService        service.IUserService    // 用户服务接口
	chatHistoryService service.IChatHistoryService
	memoryStore        store.MemoryStore
	simpleWorkflow     *workflow.SimpleWorkflow
	chatAgent          *agent.ChatAgent
	loadApp            func(context.Context, int64) (*model.App, error)
	updateArchitecture func(context.Context, int64, string) error
	updateCodeGenType  func(context.Context, int64, string) error
	db                 *gorm.DB // 数据库连接
}

func NewAppService(
	aiCodeGenFacade *core.NoCodeAIGenFacade,
	userService service.IUserService,
	chatHistoryService service.IChatHistoryService,
	db *gorm.DB,
	memoryStore store.MemoryStore,
	simpleWorkflow *workflow.SimpleWorkflow,
	chatAgent *agent.ChatAgent,
) *AppService {
	return &AppService{
		aiCodeGenFacade:    aiCodeGenFacade,
		userService:        userService,
		chatHistoryService: chatHistoryService,
		db:                 db,
		memoryStore:        memoryStore,
		simpleWorkflow:     simpleWorkflow,
		chatAgent:          chatAgent,
		loadApp: func(ctx context.Context, appID int64) (*model.App, error) {
			q := query.Use(db)
			return q.App.WithContext(ctx).
				Where(q.App.ID.Eq(appID), q.App.IsDelete.Eq(0)).
				First()
		},
		updateArchitecture: func(ctx context.Context, appID int64, architecture string) error {
			q := query.Use(db)
			_, err := q.App.WithContext(ctx).
				Where(q.App.ID.Eq(appID), q.App.IsDelete.Eq(0)).
				Update(q.App.ProjectArchitecture, architecture)
			return err
		},
		updateCodeGenType: func(ctx context.Context, appID int64, codeGenType string) error {
			q := query.Use(db)
			_, err := q.App.WithContext(ctx).
				Where(q.App.ID.Eq(appID), q.App.IsDelete.Eq(0), q.App.CodeGenType.Eq("")).
				Update(q.App.CodeGenType, codeGenType)
			return err
		},
	}
}
func (s *AppService) GetSourceCode(ctx context.Context, appId int64, generationType string, loginUser *api.UserVo) (map[string]string, error) {
	if appId <= 0 || loginUser == nil {
		return nil, errorutil.ParamsError
	}
	app, err := s.loadAppRecord(ctx, appId)
	if err != nil {
		return nil, err
	}
	if app.UserID != loginUser.ID {
		return nil, errorutil.NotAuthError
	}
	summarizing, err := s.chatHistoryService.IsSummarizing(ctx, appId)
	if err != nil {
		return nil, errorutil.SystemError.WithMessage("failed to check conversation summary status")
	}
	if summarizing {
		return nil, errorutil.ParamsError.WithMessage("conversation summary is in progress")
	}
	storedGenerationType := enum.CodeGenTypeEnum(app.CodeGenType)
	requestedGenerationType := enum.CodeGenTypeEnum(generationType)
	if storedGenerationType != "" && enum.CodeGenTypeTextMap[storedGenerationType] == "" {
		storedGenerationType = ""
	}
	if requestedGenerationType != "" && enum.CodeGenTypeTextMap[requestedGenerationType] == "" {
		requestedGenerationType = ""
	}

	projectRoot, err := file.GetProjectRoot()
	if err != nil {
		return nil, err
	}
	outputRoot, err := file.GetCodeOutputRoot()
	if err != nil {
		return nil, err
	}

	roots := []string{outputRoot}
	legacyRoot := filepath.Join(projectRoot, "saves")
	if legacyRoot != outputRoot {
		roots = append(roots, legacyRoot)
	}

	directory, err := findSourceCodeDirectory(
		roots,
		appId,
		storedGenerationType,
		requestedGenerationType,
	)
	if err != nil {
		return nil, err
	}

	files, _, err := graphnode.ReadCodeFiles(directory)
	return files, err
}

func findSourceCodeDirectory(
	roots []string,
	appID int64,
	storedGenerationType enum.CodeGenTypeEnum,
	requestedGenerationType enum.CodeGenTypeEnum,
) (string, error) {
	if appID <= 0 {
		return "", fmt.Errorf("invalid app ID: %d", appID)
	}

	types := make([]enum.CodeGenTypeEnum, 0, len(enum.CodeGenTypeTextMap))
	addType := func(codeGenType enum.CodeGenTypeEnum) {
		if codeGenType == "" || enum.CodeGenTypeTextMap[codeGenType] == "" {
			return
		}
		for _, existing := range types {
			if existing == codeGenType {
				return
			}
		}
		types = append(types, codeGenType)
	}
	addType(storedGenerationType)
	addType(requestedGenerationType)
	addType(enum.HtmlCodeGen)
	addType(enum.MultiFileGen)
	addType(enum.VueCodeGen)

	for _, root := range roots {
		for _, codeGenType := range types {
			directory := filepath.Join(root, fmt.Sprintf("%s_%d", codeGenType, appID))
			info, err := os.Stat(directory)
			if err == nil && info.IsDir() {
				return directory, nil
			}
		}
	}

	return "", fmt.Errorf("generated source code not found for app %d", appID)
}

func (s *AppService) ChatWithAgent(ctx context.Context, appId int64, message string, loginUser *api.UserVo) (*schema.StreamReader[*schema.Message], error) {
	logger.Infof("[ChatAgent] request started: appID=%d, userID=%d", appId, func() int64 {
		if loginUser == nil {
			return 0
		}
		return loginUser.ID
	}())
	if strings.TrimSpace(message) == "" || appId <= 0 || loginUser == nil {
		return nil, errorutil.ParamsError.WithMessage("invalid chat request")
	}
	if s.chatAgent == nil || s.chatHistoryService == nil || s.memoryStore == nil {
		return nil, errorutil.SystemError.WithMessage("chat service dependencies are not initialized")
	}
	app, err := s.loadAppRecord(ctx, appId)
	if err != nil {
		return nil, err
	}
	if app.UserID != loginUser.ID {
		return nil, errorutil.NotAuthError
	}
	if err := s.chatHistoryService.AddChatMessage(ctx, appId, message, enum.UserMessageType, loginUser.ID); err != nil {
		logger.Errorf("[ChatAgent] save user history failed: %v", err)
		return nil, errorutil.SystemError.WithMessage("failed to save chat history")
	}
	if err := s.memoryStore.AddUserMessage(ctx, message, strconv.FormatInt(appId, 10)); err != nil {
		logger.Errorf("[ChatAgent] save user memory failed: %v", err)
		return nil, errorutil.SystemError.WithMessage("failed to save user message to memory")
	}
	history, err := s.chatHistoryService.EnsureMemoryLoaded(ctx, appId, 20)
	if err != nil {
		logger.Errorf("[ChatAgent] load memory failed: %v", err)
		return nil, err
	}
	stream, err := s.chatAgent.Chat(
		s.withTokenUsageRecorder(ctx, appId),
		history,
	)
	if err != nil {
		logger.Errorf("[ChatAgent] create agent stream failed: %v", err)
		return nil, err
	}
	logger.Infof("[ChatAgent] stream created: appID=%d, history=%d", appId, len(history))
	return s.persistChatAgentStream(ctx, stream, appId, loginUser.ID), nil
}

func (s *AppService) persistChatAgentStream(ctx context.Context, source *schema.StreamReader[*schema.Message], appID, userID int64) *schema.StreamReader[*schema.Message] {
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer source.Close()
		defer writer.Close()
		var content strings.Builder
		for {
			msg, err := source.Recv()
			if err == io.EOF {
				text := strings.TrimSpace(content.String())
				if text != "" {
					_ = s.chatHistoryService.AddChatMessage(ctx, appID, text, enum.AIMessageType, userID)
					if err := s.memoryStore.AddAssistantMessage(ctx, text, strconv.FormatInt(appID, 10)); err != nil {
						logger.Errorf("save assistant message to memory failed: %v", err)
					}
					s.chatHistoryService.MaybeGenerateSummary(ctx, appID, userID)
				}
				return
			}
			if err != nil {
				_ = writer.Send(nil, err)
				return
			}
			if msg != nil {
				content.WriteString(msg.Content)
				if writer.Send(msg, nil) {
					return
				}
			}
		}
	}()
	return reader
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
		CodeGenType: "",
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

	logger.Infof("应用创建成功，ID: %d，代码生成类型将在首次生成时确定", appId)
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
	if s.memoryStore != nil {
		if err := s.memoryStore.ClearMessages(ctx, strconv.FormatInt(id, 10)); err != nil {
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
		ID:               app.ID,
		AppName:          app.AppName,
		Cover:            app.Cover,
		InitPrompt:       app.InitPrompt,
		CodeGenType:      app.CodeGenType,
		DeployKey:        app.DeployKey,
		DeployedTime:     app.DeployedTime,
		Priority:         app.Priority,
		UserID:           app.UserID,
		User:             *userVo,
		CreateTime:       app.CreateTime,
		UpdateTime:       app.UpdateTime,
		TokenUsage:       app.TokenUsage,
		PromptTokens:     app.PromptTokens,
		CompletionTokens: app.CompletionTokens,
	}
	s.enrichAppMemory(ctx, &appVo, app.TokenUsage)
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
			ID:               app.ID,
			AppName:          app.AppName,
			Cover:            app.Cover,
			InitPrompt:       app.InitPrompt,
			CodeGenType:      app.CodeGenType,
			DeployKey:        app.DeployKey,
			DeployedTime:     app.DeployedTime,
			Priority:         app.Priority,
			UserID:           app.UserID,
			User:             userVoMap[app.UserID],
			CreateTime:       app.CreateTime,
			UpdateTime:       app.UpdateTime,
			TokenUsage:       app.TokenUsage,
			PromptTokens:     app.PromptTokens,
			CompletionTokens: app.CompletionTokens,
		}
		s.enrichAppMemory(ctx, &appVo, app.TokenUsage)
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
	if s.memoryStore != nil {
		if err := s.memoryStore.ClearMessages(ctx, strconv.FormatInt(id, 10)); err != nil {
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
		ID:               app.ID,
		AppName:          app.AppName,
		Cover:            app.Cover,
		InitPrompt:       app.InitPrompt,
		CodeGenType:      app.CodeGenType,
		DeployKey:        app.DeployKey,
		DeployedTime:     app.DeployedTime,
		Priority:         app.Priority,
		UserID:           app.UserID,
		User:             *userVo,
		CreateTime:       app.CreateTime,
		UpdateTime:       app.UpdateTime,
		TokenUsage:       app.TokenUsage,
		PromptTokens:     app.PromptTokens,
		CompletionTokens: app.CompletionTokens,
	}
	s.enrichAppMemory(ctx, &appVo, app.TokenUsage)
	return appVo, nil
}

func (s *AppService) enrichAppMemory(ctx context.Context, appVo *api.AppVo, databaseTokenUsage int64) {
	if appVo == nil {
		return
	}
	appVo.Memory.PromptTokens = appVo.PromptTokens
	appVo.Memory.CompletionTokens = appVo.CompletionTokens
	appVo.Memory.TotalTokens = databaseTokenUsage
	if s.memoryStore == nil {
		appVo.TokenUsage = databaseTokenUsage
		return
	}
	metadata, err := s.memoryStore.GetMetadata(ctx, strconv.FormatInt(appVo.ID, 10))
	if err != nil {
		logger.Warnf("load app memory metadata failed: %v", err)
		appVo.TokenUsage = databaseTokenUsage
		return
	}
	appVo.Memory = api.MemoryVo{
		Summary:          metadata.Summary,
		Round:            metadata.Round,
		PromptTokens:     metadata.PromptTokens,
		CompletionTokens: metadata.CompletionTokens,
		TotalTokens:      metadata.TotalTokens,
		Summarizing:      metadata.Summarizing,
		SummaryError:     metadata.SummaryError,
		UpdatedAt:        metadata.UpdatedAt,
	}
	if appVo.Memory.TotalTokens == 0 {
		appVo.Memory.TotalTokens = databaseTokenUsage
	}
	if appVo.Memory.PromptTokens == 0 {
		appVo.Memory.PromptTokens = appVo.PromptTokens
	}
	if appVo.Memory.CompletionTokens == 0 {
		appVo.Memory.CompletionTokens = appVo.CompletionTokens
	}
	appVo.TokenUsage = appVo.Memory.TotalTokens
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
	if app.CodeGenType != "" && enum.CodeGenTypeTextMap[enum.CodeGenTypeEnum(app.CodeGenType)] == "" {
		return nil, errorutil.ParamsError.WithMessage("应用代码生成类型不支持")
	}

	err = s.chatHistoryService.AddChatMessage(ctx, appId, message, enum.UserMessageType, loginUser.ID)
	if err != nil {
		return nil, errorutil.SystemError.WithMessage("failed to save chat history")
	}
	// 5. 调用代码生成服务
	return s.aiCodeGenFacade.GenCodeStreamAndSave(
		ctx,
		appId,
		[]*schema.Message{schema.UserMessage(message)},
		enum.CodeGenTypeEnum(app.CodeGenType),
	)
}

func (s *AppService) GraphToGenCode(ctx context.Context, appId int64, message string, loginUser *api.UserVo) (*schema.StreamReader[*schema.Message], *state.WorkFlowContext, error) {
	if s.chatHistoryService == nil || s.memoryStore == nil || s.simpleWorkflow == nil {
		return nil, nil, errorutil.SystemError.WithMessage("graph service dependencies are not initialized")
	}
	if message == "" {
		return nil, nil, errorutil.ParamsError.WithMessage("消息不能为空")
	}
	if appId <= 0 || loginUser == nil {
		return nil, nil, errorutil.ParamsError.WithMessage("应用ID或用户不能为空")
	}
	if s.simpleWorkflow == nil {
		return nil, nil, errorutil.SystemError.WithMessage("工作流未初始化")
	}
	if s.memoryStore == nil {
		return nil, nil, errorutil.SystemError.WithMessage("memory store未初始化")
	}

	app, err := s.loadAppRecord(ctx, appId)
	if err != nil {
		return nil, nil, err
	}
	if app.UserID != loginUser.ID {
		return nil, nil, errorutil.NotAuthError.WithMessage("无权使用该应用")
	}

	if app.CodeGenType != "" && enum.CodeGenTypeTextMap[enum.CodeGenTypeEnum(app.CodeGenType)] == "" {
		return nil, nil, errorutil.ParamsError.WithMessage("应用代码生成类型不支持")
	}
	summarizing, err := s.chatHistoryService.IsSummarizing(ctx, appId)
	if err != nil {
		return nil, nil, errorutil.SystemError.WithMessage("failed to check conversation summary status")
	}
	if summarizing {
		return nil, nil, errorutil.ParamsError.WithMessage("conversation summary is in progress")
	}

	var unlock func()
	if locker, ok := s.memoryStore.(store.DistributedLocker); ok {
		unlock, err = locker.Lock(ctx, strconv.FormatInt(appId, 10))
		if err != nil {
			return nil, nil, errorutil.SystemError.WithMessage("failed to acquire application generation lock")
		}
	}

	if err := s.chatHistoryService.AddChatMessage(ctx, appId, message, enum.UserMessageType, loginUser.ID); err != nil {
		if unlock != nil {
			unlock()
		}
		return nil, nil, errorutil.SystemError.WithMessage("failed to save chat history")
	}
	if err := s.memoryStore.AddUserMessage(ctx, message, strconv.FormatInt(appId, 10)); err != nil {
		if unlock != nil {
			unlock()
		}
		return nil, nil, errorutil.SystemError.WithMessage("failed to save user message to memory")
	}

	history, err := s.chatHistoryService.EnsureMemoryLoaded(ctx, appId, 20)
	if err != nil {
		if unlock != nil {
			unlock()
		}
		return nil, nil, errorutil.SystemError.WithMessage("failed to load workflow memory")
	}
	projectArchitecture := app.ProjectArchitecture
	originalPrompt := buildWorkflowPrompt(history, projectArchitecture, "")

	workflowContext := &state.WorkFlowContext{
		AppID:          appId,
		OriginalPrompt: originalPrompt,
		GenerationType: enum.CodeGenTypeEnum(app.CodeGenType),
		MaxRetries:     3,
	}

	stream, err := s.simpleWorkflow.ExecuteStream(
		s.withTokenUsageRecorder(ctx, appId),
		workflowContext,
	)
	if err != nil {
		if unlock != nil {
			unlock()
		}
		return nil, nil, err
	}
	return s.persistWorkflowContextStream(ctx, stream, workflowContext, appId, loginUser.ID, unlock), workflowContext, nil
}

func (s *AppService) persistWorkflowContextStream(
	ctx context.Context,
	source *schema.StreamReader[*schema.Message],
	workflowContext *state.WorkFlowContext,
	appID int64,
	userID int64,
	unlock func(),
) *schema.StreamReader[*schema.Message] {
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer source.Close()
		defer writer.Close()
		if unlock != nil {
			defer unlock()
		}
		for {
			message, err := source.Recv()
			if err == io.EOF {
				if ctx.Err() != nil {
					return
				}
				if workflowContext.GenerationType != "" && s.updateCodeGenType != nil {
					if err := s.updateCodeGenType(ctx, appID, string(workflowContext.GenerationType)); err != nil {
						logger.Errorf("更新代码生成类型失败: %v", err)
					}
				}
				s.saveWorkflowDescription(ctx, workflowContext, appID, userID)
				s.chatHistoryService.MaybeGenerateSummary(ctx, appID, userID)
				return
			}
			if err != nil {
				_ = writer.Send(nil, err)
				return
			}
			if writer.Send(message, nil) {
				return
			}
		}
	}()
	return reader
}

func (s *AppService) saveWorkflowDescription(ctx context.Context, workflowContext *state.WorkFlowContext, appID, userID int64) {
	description := strings.TrimSpace(workflowContext.Description)
	if description != "" {
		if err := s.chatHistoryService.AddChatMessage(ctx, appID, description, enum.AIMessageType, userID); err != nil {
			logger.Errorf("保存生成描述到 MySQL 失败: %v", err)
		}
		if err := s.memoryStore.AddAssistantMessage(ctx, description, strconv.FormatInt(appID, 10)); err != nil {
			logger.Errorf("保存生成描述到 memory store 失败: %v", err)
		}
	}

	architecture := buildProjectArchitecture(workflowContext.CodeContent)
	if architecture == "" {
		return
	}
	if err := s.updateAppArchitecture(ctx, appID, architecture); err != nil {
		logger.Errorf("更新项目架构失败: %v", err)
	}
}

func (s *AppService) loadAppRecord(ctx context.Context, appID int64) (*model.App, error) {
	if s.loadApp == nil {
		return nil, errorutil.SystemError.WithMessage("应用查询未初始化")
	}
	return s.loadApp(ctx, appID)
}

func (s *AppService) updateAppArchitecture(ctx context.Context, appID int64, architecture string) error {
	if s.updateArchitecture == nil {
		return errorutil.SystemError.WithMessage("应用架构更新未初始化")
	}
	return s.updateArchitecture(ctx, appID, architecture)
}

func buildProjectArchitecture(codeContent map[string]string) string {
	paths := make([]string, 0, len(codeContent))
	for path := range codeContent {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return strings.Join(paths, "\n")
}

func buildWorkflowPrompt(history []*schema.Message, architecture, message string) string {
	var builder strings.Builder
	if architecture != "" {
		builder.WriteString("## 项目架构\n")
		builder.WriteString(architecture)
		builder.WriteString("\n\n")
	}
	for _, item := range history {
		if item == nil || item.Content == "" {
			continue
		}
		builder.WriteString(string(item.Role))
		builder.WriteString(": ")
		builder.WriteString(item.Content)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## 本次用户需求\n")
	builder.WriteString(message)
	return builder.String()
}

// 让ctx带上recorder，baseagent取recorder，recorder的记录逻辑在service层
func (s *AppService) withTokenUsageRecorder(ctx context.Context, appID int64) context.Context {
	return agent.WithTokenUsageRecorder(ctx, &appTokenUsageRecorder{
		service: s,
		appID:   appID,
	})
}
