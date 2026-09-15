package agent

import (
	"context"
	"strconv"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/core/store"
	"github.com/chosenlau/noCodeAI/internal/service"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/patrickmn/go-cache"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// CodeGenAgentFactory with cache and redis memory
// MaxAgentInstances 最大Agent实例数量
const MaxAgentInstances = 1000

var (
	// serviceCache 全局缓存，存储Agent实例
	// 默认过期时间：30分钟
	// 清理间隔：10分钟
	serviceCache = cache.New(30*time.Minute, 10*time.Minute)
)

type CodeGenAgentFactory struct {
	chatModel          ChatModelWrapperAdaptor
	redisClient        *redis.Client
	chatHistoryService service.IChatHistoryService
	sfGroup            singleflight.Group
}

func NewCodeGenAgentFactory(
	chatModel ChatModelWrapperAdaptor,
	redisClient *redis.Client,
	chatHistoryService service.IChatHistoryService,
) *CodeGenAgentFactory {
	// 注册淘汰回调，记录日志
	serviceCache.OnEvicted(func(k string, v interface{}) {
		logger.Debugf("AI服务实例被移除，缓冲键: %v", k)
	})

	return &CodeGenAgentFactory{
		chatModel:          chatModel,
		redisClient:        redisClient,
		chatHistoryService: chatHistoryService,
	}
}

func (c *CodeGenAgentFactory) GetCodeGenAgent(ctx context.Context, appId int64, codeGenType enum.CodeGenTypeEnum) (*CodeGenAgent, error) {
	// 1. 构建缓存Key
	key := buildCacheKey(appId, codeGenType)
	// 2. 尝试从缓存获取
	if agent, found := serviceCache.Get(key); found {
		// 缓存命中，直接返回
		return agent.(*CodeGenAgent), nil
	}
	// 3. 缓存未命中，检查实例数量
	v, err, _ := c.sfGroup.Do(key, func() (interface{}, error) {

		// 3. Double Check (双重检查)：极其重要！
		// 第 1 个拿到锁的执行完后，其他阻塞的协程被唤醒，如果这里不查一次，它们还是会再走一遍创建流程。
		if agent, found := serviceCache.Get(key); found {
			return agent.(*CodeGenAgent), nil
		}
		// 4. 执行容量淘汰 (依然需要注意 evictOldest 的并发安全，见上一问)
		if serviceCache.ItemCount() >= MaxAgentInstances {
			c.evictOldest()
		}
		// 5. 延迟初始化，创建真正需要的对象
		redisStore := store.NewRedisMemoryStore(
			c.redisClient,
			strconv.Itoa(int(appId)),
			20,
			24*time.Hour,
		)
		_, err := c.chatHistoryService.LoadChatHistoryToMemory(
			ctx,
			appId,
			redisStore,
			20,
		)
		if err != nil {
			return nil, err
		}
		agent := NewCodeGenAgent(c.chatModel, codeGenType, redisStore)

		// 6. 放入缓存
		serviceCache.Set(key, agent, cache.DefaultExpiration)
		return agent, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*CodeGenAgent), nil
}

func buildCacheKey(appId int64, codeGenType enum.CodeGenTypeEnum) string {
	return strconv.Itoa(int(appId)) + "_" + string(codeGenType)
}

// evictOldest 淘汰最老的缓存项（LRU策略）
func (c *CodeGenAgentFactory) evictOldest() {
	items := serviceCache.Items()
	oldestKey := ""
	var oldestExpiration int64

	// 遍历所有缓存项，找到过期时间最早的
	for k, item := range items {
		if item.Expiration == 0 {
			continue // 永不过期的项，跳过
		}
		if oldestKey == "" || item.Expiration < oldestExpiration {
			oldestExpiration = item.Expiration
			oldestKey = k
		}
	}

	// 删除最老的项
	if oldestKey != "" {
		serviceCache.Delete(oldestKey)
	}
}
