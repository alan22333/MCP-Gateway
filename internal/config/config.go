// Package config 使用 viper 加载 config.yaml 配置文件
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config 应用程序配置结构体
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
}

// ServerConfig 服务器相关配置
type ServerConfig struct {
	Port int `mapstructure:"port"` // 监听端口
}

// DatabaseConfig 数据库相关配置
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"` // 数据库驱动: sqlite / mysql
	DSN    string `mapstructure:"dsn"`    // 数据库连接字符串
}

// Load 从 config.yaml 加载配置
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".") // 从当前目录搜索 config.yaml

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}
