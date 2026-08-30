package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/bytedance/gopkg/util/logger"
	"github.com/spf13/viper"
)

// mapstructure->viper配置
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	AI       AIConfig       `mapstructure:"ai"`
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

type AIConfig struct {
	APIKey  string `mapstructure:"api_key"`
	Model   string `mapstructure:"model"`
	BaseURL string `mapstructure:"base_url"`
}

var GlobalConfig *Config

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

func InitConfig(env string) {
	rootPath, err := GetProjectRootPath()
	if err != nil {
		panic(err)
	}
	cfgName := fmt.Sprintf("config-%s.yml", env)
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
}

func (c *Config) GetDatabaseDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.Database.Username,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Database)
}
