// Package config 使用 viper 加载 config.yaml 配置文件
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// Config 应用程序配置结构体
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Cache          CacheConfig          `mapstructure:"cache"`
	RateLimit      RateLimitConfig      `mapstructure:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Auth           AuthConfig            `mapstructure:"auth"`
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
	Enabled       bool   `mapstructure:"enabled"`
	RedisAddr     string `mapstructure:"redis_addr"`
	RedisPassword string `mapstructure:"redis_password"`
	RedisDB       int    `mapstructure:"redis_db"`
	TTL           int    `mapstructure:"ttl"`
}

// RateLimitConfig 令牌桶限流配置
type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"` // 每秒允许的请求数
	Burst             int     `mapstructure:"burst"`               // 突发令牌数
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	Enabled              bool `mapstructure:"enabled"`
	MaxFailures          int  `mapstructure:"max_failures"`
	Timeout              int  `mapstructure:"timeout"`
	HalfOpenMaxRequests  int  `mapstructure:"half_open_max_requests"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled     bool     `mapstructure:"enabled"`      // 是否启用 API Key 认证
	ExemptPaths []string `mapstructure:"exempt_paths"` // 豁免路径前缀（如 /metrics, /）
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
