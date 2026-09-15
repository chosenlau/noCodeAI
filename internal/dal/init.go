package dal

import (
	"fmt"

	config "github.com/chosenlau/noCodeAI/config"
	"github.com/redis/go-redis/v9"
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
		Addr:     fmt.Sprintf("%s%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
}
