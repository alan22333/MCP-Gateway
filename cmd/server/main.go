// MCP Gateway 服务入口：依赖组装、路由注册、启停控制
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
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

	// ── 1. 配置 & 数据库 ──
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

	// ── 2. 核心依赖 ──
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
	// 每 session 并发控制（默认 5 个并发槽位）
	sessionMgr.SetConcurrencyLimit(5)
	logger.Info("并发控制已启用", zap.Int("max_concurrent_per_session", 5))

	// 运行时可热更新的配置
	rtCfg := &middleware.RuntimeConfig{AuthEnabled: cfg.Auth.Enabled}
	var rtCfgPtr atomic.Value
	rtCfgPtr.Store(rtCfg)

	// ── 3. 配置热更新 ──
	done := make(chan struct{})
	defer close(done)
	config.WatchAndReload(func(newCfg *config.Config) {
		// 更新 session 管理器限流参数
		if newCfg.RateLimit.Enabled {
			sessionMgr.SetRateLimit(newCfg.RateLimit.RequestsPerSecond, newCfg.RateLimit.Burst)
		}
		// 更新运行时配置（auth 开关）
		rtCfg.AuthEnabled = newCfg.Auth.Enabled
		rtCfgPtr.Store(rtCfg)
		logger.Info("配置已热更新",
			zap.Float64("rps", newCfg.RateLimit.RequestsPerSecond),
			zap.Bool("auth_enabled", newCfg.Auth.Enabled))
	}, done)
	logger.Info("配置文件监控已启动（热更新就绪）")

	// ── 4. 路由（分组中间件栈）──
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 全局中间件：TraceID 和请求体大小限制对所有路由生效
	r.Use(middleware.TraceID())
	r.Use(middleware.BodyLimit(int64(cfg.Server.MaxBodyBytes)))

	// ── 分组 1：可观测 & 健康检查（无需认证）──
	publicGroup := r.Group("")
	{
		publicGroup.GET("/metrics", gin.WrapH(promhttp.Handler()))
		publicGroup.GET("/api/health", handler.NewHealthHandler(httpProxy).Check)
	}

	// ── 分组 2：MCP 协议端点（认证中间件）──
	mcpGroup := r.Group("/mcp", middleware.APIKeyAuth(middleware.AuthConfig{
		Enabled: cfg.Auth.Enabled, ExemptPaths: cfg.Auth.ExemptPaths,
	}, apiRepo, &rtCfgPtr))
	{
		if _, err = apiRepo.EnsureDefaultGateway(); err != nil {
			logger.Warn("默认网关迁移失败", zap.Error(err))
		}

		mcpHandler := handler.NewMcpHandler(sessionMgr, apiRepo, svc, logger)
		// 旧版 SSE 传输（向后兼容）
		mcpGroup.GET("/sse", mcpHandler.HandleSSE)
		mcpGroup.POST("/message", mcpHandler.HandleMessage)

		// 新版 Streamable HTTP 传输（MCP 2025 spec）
		streamableHandler := handler.NewStreamableHandler(sessionMgr, apiRepo, svc, logger)
		mcpGroup.POST("", streamableHandler.Handle) // POST /mcp
	}

	// 启动 session 过期清理（30 分钟未活跃则移除）
	go sessionMgr.CleanupExpired(30*time.Minute, done)
	logger.Info("会话过期清理已启动", zap.Duration("ttl", 30*time.Minute))

	// ── 分组 3：管理 API（认证中间件）──
	apiGroup := r.Group("/api", middleware.APIKeyAuth(middleware.AuthConfig{
		Enabled: cfg.Auth.Enabled, ExemptPaths: cfg.Auth.ExemptPaths,
	}, apiRepo, &rtCfgPtr))
	{
		handler.NewGatewayHandler(apiRepo).RegisterRoutes(apiGroup)
		handler.NewToolHandler(apiRepo, svc).RegisterRoutes(apiGroup)
		handler.NewImportHandler(apiRepo).RegisterRoutes(apiGroup)
		handler.NewLogHandler(apiRepo).RegisterRoutes(apiGroup)
		handler.NewSessionHandler(sessionMgr).RegisterRoutes(apiGroup)

		keyHandler := handler.NewKeyHandler(apiRepo)
		keyHandler.RegisterRoutes(apiGroup)
		if cfg.Auth.Enabled {
			keyHandler.SeedDefault(logger)
		}
	}

	// ── 分组 4：管理后台前端（无需认证）──
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/index.html", "./web/index.html")
	r.Static("/static", "./web/static")

	// ── 5. 启动 & 优雅关闭 ──
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
