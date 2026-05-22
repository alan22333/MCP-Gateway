//go:build wireinject
// +build wireinject

// Wire 依赖注入定义文件，运行 `wire` 生成 wire_gen.go
package main

import (
	"time"

	"github.com/alan22333/mcp-nexus/internal/cache"
	"github.com/alan22333/mcp-nexus/internal/config"
	"github.com/alan22333/mcp-nexus/internal/handler"
	"github.com/alan22333/mcp-nexus/internal/proxy"
	"github.com/alan22333/mcp-nexus/internal/repository"
	"github.com/alan22333/mcp-nexus/internal/service"

	"github.com/google/wire"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ── Provider Sets ──

var configSet = wire.NewSet(config.Load)

var loggerSet = wire.NewSet(
	func() (*zap.Logger, error) {
		logger, err := zap.NewProduction()
		if err != nil {
			return nil, err
		}
		return logger, nil
	},
)

var dbSet = wire.NewSet(
	func(cfg *config.Config) (*gorm.DB, error) {
		db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), &gorm.Config{
			Logger: gormlogger.Default.LogMode(gormlogger.Warn),
		})
		if err != nil {
			return nil, err
		}
		return db, nil
	},
)

var repoSet = wire.NewSet(repository.NewApiToolRepo)

var proxySet = wire.NewSet(proxy.NewHttpProxy)

var cbSet = wire.NewSet(
	func(cfg *config.Config) *proxy.CircuitBreakerManager {
		return proxy.NewCircuitBreakerManager(proxy.CBConfig{
			MaxFailures:         cfg.CircuitBreaker.MaxFailures,
			Timeout:             time.Duration(cfg.CircuitBreaker.Timeout) * time.Second,
			HalfOpenMaxRequests: cfg.CircuitBreaker.HalfOpenMaxRequests,
		})
	},
)

var cacheSet = wire.NewSet(
	func(cfg *config.Config) cache.ToolCache {
		if cfg.Cache.Enabled && cfg.Cache.RedisAddr != "" {
			redisCache, err := cache.NewRedisCache(cache.RedisConfig{
				Addr: cfg.Cache.RedisAddr, Password: cfg.Cache.RedisPassword, DB: cfg.Cache.RedisDB,
			})
			if err == nil {
				return redisCache
			}
		}
		return cache.NewMemCache()
	},
)

var serviceSet = wire.NewSet(
	func(repo *repository.ApiToolRepo, p *proxy.HttpProxy, cb *proxy.CircuitBreakerManager, c cache.ToolCache, cfg *config.Config, logger *zap.Logger) *service.McpService {
		cacheTTL := time.Duration(cfg.Cache.TTL) * time.Second
		return service.NewMcpService(repo, p, nil, cb, c, cacheTTL, logger)
	},
)

var sessionSet = wire.NewSet(handler.NewSessionManager)

var mcpHandlerSet = wire.NewSet(
	func(sessionMgr *handler.SessionManager, repo *repository.ApiToolRepo, svc *service.McpService, logger *zap.Logger) *handler.McpHandler {
		return handler.NewMcpHandler(sessionMgr, repo, svc, logger)
	},
)

// InitializeApp Wire 生成的依赖注入函数
func InitializeApp() (*App, error) {
	wire.Build(
		configSet, loggerSet, dbSet, repoSet, proxySet, cbSet,
		cacheSet, serviceSet, sessionSet, mcpHandlerSet,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}
