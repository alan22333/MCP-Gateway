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
	Cache    CacheConfig    `mapstructure:"cache"`
}

// ServerConfig 服务器相关配置
type ServerConfig struct {
	Port int `mapstructure:"port"`
}

// DatabaseConfig 数据库相关配置
type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

// CacheConfig 缓存相关配置
type CacheConfig struct {
	Enabled       bool   `mapstructure:"enabled"`        // 是否启用缓存
	RedisAddr     string `mapstructure:"redis_addr"`     // Redis 地址，如 "localhost:6379"
	RedisPassword string `mapstructure:"redis_password"` // Redis 密码
	RedisDB       int    `mapstructure:"redis_db"`       // Redis DB 编号
	TTL           int    `mapstructure:"ttl"`            // 缓存过期时间（秒）
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
