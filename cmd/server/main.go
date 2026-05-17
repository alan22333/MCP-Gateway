// MCP Gateway 服务入口：依赖组装、路由注册、启停控制
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"mcp-gateway-go-demo/internal/cache"
	"mcp-gateway-go-demo/internal/config"
	"mcp-gateway-go-demo/internal/handler"
	"mcp-gateway-go-demo/internal/middleware"
	"mcp-gateway-go-demo/internal/proxy"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// 1. 配置 & 数据库
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("加载配置失败", zap.Error(err))
	}

	db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		logger.Fatal("数据库连接失败", zap.Error(err))
	}
	logger.Info("数据库连接成功")

	apiRepo := repository.NewApiToolRepo(db)
	if err := apiRepo.AutoMigrate(); err != nil {
		logger.Fatal("数据库自动建表失败", zap.Error(err))
	}
	logger.Info("数据库表结构就绪")

	// 2. 核心依赖
	httpProxy := proxy.NewHttpProxy()

	cbManager := proxy.NewCircuitBreakerManager(proxy.CBConfig{
		MaxFailures:         cfg.CircuitBreaker.MaxFailures,
		Timeout:             time.Duration(cfg.CircuitBreaker.Timeout) * time.Second,
		HalfOpenMaxRequests: cfg.CircuitBreaker.HalfOpenMaxRequests,
	})
	if cfg.CircuitBreaker.Enabled {
		logger.Info("熔断器已启用",
			zap.Int("max_failures", cfg.CircuitBreaker.MaxFailures),
			zap.Int("timeout_sec", cfg.CircuitBreaker.Timeout))
	}

	var toolCache cache.ToolCache
	cacheTTL := time.Duration(cfg.Cache.TTL) * time.Second
	if cfg.Cache.Enabled && cfg.Cache.RedisAddr != "" {
		redisCache, err := cache.NewRedisCache(cache.RedisConfig{
			Addr: cfg.Cache.RedisAddr, Password: cfg.Cache.RedisPassword, DB: cfg.Cache.RedisDB,
		})
		if err != nil {
			logger.Warn("Redis 连接失败，回退到内存缓存", zap.Error(err))
			toolCache = cache.NewMemCache()
		} else {
			toolCache = redisCache
			logger.Info("Redis 缓存已连接", zap.String("addr", cfg.Cache.RedisAddr))
		}
	} else {
		toolCache = cache.NewMemCache()
		logger.Info("使用内存缓存（未配置 Redis）")
	}

	svc := service.NewMcpService(apiRepo, httpProxy, cbManager, toolCache, cacheTTL, logger)

	sessionMgr := handler.NewSessionManager()
	if cfg.RateLimit.Enabled {
		sessionMgr.SetRateLimit(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
		logger.Info("限流已启用",
			zap.Float64("rps", cfg.RateLimit.RequestsPerSecond),
			zap.Int("burst", cfg.RateLimit.Burst))
	}

	// 3. 路由
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(middleware.TraceID())
	r.Use(middleware.APIKeyAuth(middleware.AuthConfig{
		Enabled: cfg.Auth.Enabled, ExemptPaths: cfg.Auth.ExemptPaths,
	}, apiRepo))

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// MCP 协议端点
	mcpHandler := handler.NewMcpHandler(sessionMgr, svc, logger)
	r.GET("/mcp/sse", mcpHandler.HandleSSE)
	r.POST("/mcp/message", mcpHandler.HandleMessage)

	// 业务 API handlers
	handler.NewToolHandler(apiRepo, svc).RegisterRoutes(r)
	handler.NewImportHandler(apiRepo).RegisterRoutes(r)
	handler.NewLogHandler(apiRepo).RegisterRoutes(r)
	handler.NewSessionHandler(sessionMgr).RegisterRoutes(r)
	handler.NewHealthHandler(httpProxy).RegisterRoutes(r)

	keyHandler := handler.NewKeyHandler(apiRepo)
	keyHandler.RegisterRoutes(r)
	if cfg.Auth.Enabled {
		keyHandler.SeedDefault(logger)
	}

	// 管理后台前端
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/index.html", "./web/index.html")
	r.Static("/static", "./web/static")

	// 4. 启动 & 优雅关闭
	addr := ":" + strconv.Itoa(cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	go func() {
		logger.Info("MCP Gateway 启动", zap.String("address", addr),
			zap.String("dashboard", "http://localhost"+addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("服务异常退出", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("收到退出信号，开始优雅关闭...", zap.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("服务关闭超时", zap.Error(err))
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}
	logger.Info("服务已安全退出")
}
