package dal

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	config "github.com/chosenlau/noCodeAI/config"
	"github.com/redis/go-redis/v9"
	"github.com/tencentyun/cos-go-sdk-v5"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func InitDB(cfg *config.Config) *gorm.DB {
	if cfg == nil {
		panic("config is nil")
	}
	dsn := cfg.GetDatabaseDSN()

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic(fmt.Errorf("failed to connect to database: %v", err))
	}
	return db
}

func InitRedis(cfg *config.Config) *redis.Client {
	if cfg == nil {
		panic("config is nil")
	}
	return redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}

func InitCOSClient(config *config.Config) *cos.Client {
	if config == nil {
		panic(fmt.Errorf("配置加载失败"))
	}

	bucketURL, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", config.COS.Bucket, config.COS.Region))
	if err != nil {
		panic(fmt.Errorf("解析COS URL失败: %w", err))
	}

	baseURL := &cos.BaseURL{
		BucketURL: bucketURL,
	}

	client := cos.NewClient(baseURL, &http.Client{
		Timeout: 100 * time.Second,
		Transport: &cos.AuthorizationTransport{
			SecretID:  config.COS.SecretID,
			SecretKey: config.COS.SecretKey,
		},
	})

	return client
}
