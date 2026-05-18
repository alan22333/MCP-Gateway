// Package config 使用 viper 加载 config.yaml，支持热更新和环境变量覆盖
package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Config 应用程序配置结构体
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Database       DatabaseConfig       `mapstructure:"database"`
	Cache          CacheConfig          `mapstructure:"cache"`
	RateLimit      RateLimitConfig      `mapstructure:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Auth           AuthConfig           `mapstructure:"auth"`
}

// ServerConfig 服务器相关配置
type ServerConfig struct {
	Port         int `mapstructure:"port"`
	MaxBodyBytes int `mapstructure:"max_body_bytes"` // 请求体最大字节数，默认 1MB
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
	Enabled             bool `mapstructure:"enabled"`
	MaxFailures         int  `mapstructure:"max_failures"`
	Timeout             int  `mapstructure:"timeout"`
	HalfOpenMaxRequests int  `mapstructure:"half_open_max_requests"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled     bool     `mapstructure:"enabled"`      // 是否启用 API Key 认证
	ExemptPaths []string `mapstructure:"exempt_paths"` // 豁免路径前缀（如 /metrics, /）
}

// OnChangeCallback 配置变更回调，传入新配置
type OnChangeCallback func(*Config)

var mu sync.RWMutex

// Load 从 config.yaml 加载配置
// 优先使用环境变量 MCP_GATEWAY_CONFIG 指定的路径，否则从当前目录搜索
func Load() (*Config, error) {
	configPath := os.Getenv("MCP_GATEWAY_CONFIG")
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
	}

	// 环境变量覆盖（优先级高于配置文件）
	viper.SetEnvPrefix("MCP")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = 1 << 20 // 默认 1MB
	}

	return &cfg, nil
}

// WatchAndReload 监听配置文件变更，变更后调用 callback
// 在独立的 goroutine 中运行，通过 done channel 控制退出
func WatchAndReload(callback OnChangeCallback, done <-chan struct{}) {
	viper.OnConfigChange(func(e fsnotify.Event) {
		mu.Lock()
		defer mu.Unlock()

		var newCfg Config
		if err := viper.Unmarshal(&newCfg); err != nil {
			return // 解析失败时忽略，保留旧配置
		}
		if newCfg.Server.MaxBodyBytes == 0 {
			newCfg.Server.MaxBodyBytes = 1 << 20
		}
		callback(&newCfg)
	})
	viper.WatchConfig()

	go func() {
		<-done
	}()
}
