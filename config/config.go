package config

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/chosenlau/noCodeAI/internal/core/contextmgr"
	"github.com/spf13/viper"
)

// mapstructure->viper配置
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	AI        AIConfig        `mapstructure:"ai"`
	COS       COSConfig       `yaml:"cos" mapstructure:"cos"`
	Pexels    PexelsConfig    `yaml:"pexels" mapstructure:"pexels"`
	DashScope DashScopeConfig `yaml:"dashscope" mapstructure:"dashscope"`
	Features  FeatureConfig   `yaml:"features" mapstructure:"features"`
	Task      TaskConfig      `yaml:"task_runtime" mapstructure:"task_runtime"`
}

type ServerConfig struct {
	Port        int    `mapstructure:"port"`
	ContextPath string `mapstructure:"context_path"`
}
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

type RedisConfig struct {
	Host     string `yaml:"host" mapstructure:"host"`
	Port     int    `yaml:"port" mapstructure:"port"`
	Password string `yaml:"password" mapstructure:"password"`
	DB       int    `yaml:"db" mapstructure:"db"`
}

type AIConfig struct {
	APIKey   string             `mapstructure:"api_key"`
	Model    string             `mapstructure:"model"`
	BaseURL  string             `mapstructure:"base_url"`
	Provider string             `mapstructure:"provider"`
	Context  contextmgr.Options `mapstructure:"context"`
}

type COSConfig struct {
	Host      string `yaml:"host" mapstructure:"host"`
	SecretID  string `yaml:"secret-id" mapstructure:"secret-id"`
	SecretKey string `yaml:"secret-key" mapstructure:"secret-key"`
	Region    string `yaml:"region" mapstructure:"region"`
	Bucket    string `yaml:"bucket" mapstructure:"bucket"`
}

type PexelsConfig struct {
	APIKey string `yaml:"api-key" mapstructure:"api-key"`
}

type DashScopeConfig struct {
	APIKey     string `yaml:"api-key" mapstructure:"api-key"`
	ImageModel string `yaml:"image-model" mapstructure:"image-model"`
}

type FeatureConfig struct {
	DetachedGraphTask bool `yaml:"detached_graph_task" mapstructure:"detached_graph_task"`
}

type TaskConfig struct {
	WorkerCount     int           `yaml:"worker_count" mapstructure:"worker_count"`
	PollInterval    time.Duration `yaml:"poll_interval" mapstructure:"poll_interval"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"`
	OutboxBatchSize int           `yaml:"outbox_batch_size" mapstructure:"outbox_batch_size"`
}

var (
	GlobalConfig *Config
	once         sync.Once
)

func GetProjectRootPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get caller information")
	}
	dir := filepath.Dir(file)
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}
		parentdir := filepath.Dir(dir)
		if parentdir == dir {
			return "", fmt.Errorf("project root not found")
		}
		dir = parentdir
	}
}
func GetProjectSavePath() (string, error) {
	root, err := GetProjectRootPath()
	if err != nil {
		return "", fmt.Errorf("获取存储根目录失败: %w", err)
	}
	genPath := path.Join(root, "saves")
	return genPath, nil
}

func InitConfig() *Config {
	once.Do(func() {
		env := flag.String("env", "", "executing environment:local,dev,test")

		flag.Parse()
		if env == nil || *env == "" {
			*env = "local"
		}
		rootPath, err := GetProjectRootPath()
		if err != nil {
			panic(err)
		}
		cfgName := fmt.Sprintf("config-%s.yml", *env)
		cfgPath := filepath.Join(rootPath, "config", cfgName)

		viper.SetConfigFile(cfgPath)
		viper.SetConfigType("yaml")

		viper.AutomaticEnv()
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Errorf("failed to read config file: %v", err))
		}

		logger.Infof("config path: %s", viper.ConfigFileUsed())

		GlobalConfig = &Config{}

		if err := viper.Unmarshal(GlobalConfig); err != nil {
			panic(fmt.Errorf("failed to unmarshal config: %v", err))
		}
	})

	return GlobalConfig
}

func (c *Config) GetDatabaseDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.Database.Username,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Database)
}
