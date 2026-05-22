package main

import (
	"sync/atomic"
	"time"

	"mcp-gateway-go-demo/internal/cache"
	"mcp-gateway-go-demo/internal/config"
	"mcp-gateway-go-demo/internal/handler"
	"mcp-gateway-go-demo/pkg/openapi"
	"mcp-gateway-go-demo/pkg/protobuf"
	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/middleware"
	"mcp-gateway-go-demo/internal/proxy"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/internal/service"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// dependencies 聚合所有核心依赖，由 bootstrap 函数装配
type dependencies struct {
	cfg          *config.Config
	logger       *zap.Logger
	db           *gorm.DB
	apiRepo      *repository.ApiToolRepo
	httpProxy    *proxy.HttpProxy
	grpcProxy    *proxy.GrpcProxy
	cbManager    *proxy.CircuitBreakerManager
	svc          *service.McpService
	sessionMgr   *handler.SessionManager
	mcpHandler   *handler.McpHandler
	streamHandler *handler.StreamableHandler
	rtCfgPtr     atomic.Value
	done         chan struct{}
}

// bootstrap 装配所有依赖，返回聚合后的依赖集
func bootstrap() (*dependencies, error) {
	// 1. 配置
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	// 2. 日志
	logger, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	// 3. 数据库
	db, err := gorm.Open(sqlite.Open(cfg.Database.DSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		logger.Fatal("database connection failed", zap.Error(err))
	}
	logger.Info("database connected")

	apiRepo := repository.NewApiToolRepo(db)
	if err := apiRepo.AutoMigrate(); err != nil {
		logger.Fatal("auto migration failed", zap.Error(err))
	}
	logger.Info("database schema ready")

	// 4. 自动注册配置文件中声明的后端服务
	grpcProxy := proxy.NewGrpcProxy()
	registerBackends(cfg, apiRepo, grpcProxy, logger)

	// 5. 核心依赖
	httpProxy := proxy.NewHttpProxy()

	cbManager := proxy.NewCircuitBreakerManager(proxy.CBConfig{
		MaxFailures:         cfg.CircuitBreaker.MaxFailures,
		Timeout:             time.Duration(cfg.CircuitBreaker.Timeout) * time.Second,
		HalfOpenMaxRequests: cfg.CircuitBreaker.HalfOpenMaxRequests,
	})
	if cfg.CircuitBreaker.Enabled {
		logger.Info("circuit breaker enabled",
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
			logger.Warn("redis connection failed, falling back to memory cache", zap.Error(err))
			toolCache = cache.NewMemCache()
		} else {
			toolCache = redisCache
			logger.Info("redis cache connected", zap.String("addr", cfg.Cache.RedisAddr))
		}
	} else {
		toolCache = cache.NewMemCache()
		logger.Info("using memory cache (no redis configured)")
	}

	svc := service.NewMcpService(apiRepo, httpProxy, grpcProxy, cbManager, toolCache, cacheTTL, logger)

	sessionMgr := handler.NewSessionManager()
	if cfg.RateLimit.Enabled {
		sessionMgr.SetRateLimit(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)
		logger.Info("rate limit enabled",
			zap.Float64("rps", cfg.RateLimit.RequestsPerSecond),
			zap.Int("burst", cfg.RateLimit.Burst))
	}
	sessionMgr.SetConcurrencyLimit(5)
	logger.Info("concurrency control enabled", zap.Int("max_per_session", 5))

	// 5. 运行时配置（热更新用）
	rtCfg := &middleware.RuntimeConfig{AuthEnabled: cfg.Auth.Enabled}
	var rtCfgPtr atomic.Value
	rtCfgPtr.Store(rtCfg)

	// 6. 配置热更新
	done := make(chan struct{})
	config.WatchAndReload(func(newCfg *config.Config) {
		if newCfg.RateLimit.Enabled {
			sessionMgr.SetRateLimit(newCfg.RateLimit.RequestsPerSecond, newCfg.RateLimit.Burst)
		}
		rtCfg.AuthEnabled = newCfg.Auth.Enabled
		rtCfgPtr.Store(rtCfg)
		logger.Info("config hot-reloaded",
			zap.Float64("rps", newCfg.RateLimit.RequestsPerSecond),
			zap.Bool("auth_enabled", newCfg.Auth.Enabled))
	}, done)
	logger.Info("config file watching started (hot reload ready)")

	// 7. MCP handlers
	mcpHandler := handler.NewMcpHandler(sessionMgr, apiRepo, svc, logger)
	streamHandler := handler.NewStreamableHandler(sessionMgr, apiRepo, svc, logger)

	return &dependencies{
		cfg:          cfg,
		logger:       logger,
		db:           db,
		apiRepo:      apiRepo,
		httpProxy:    httpProxy,
		grpcProxy:    grpcProxy,
		cbManager:    cbManager,
		svc:          svc,
		sessionMgr:   sessionMgr,
		mcpHandler:   mcpHandler,
		streamHandler: streamHandler,
		rtCfgPtr:     rtCfgPtr,
		done:         done,
	}, nil
}

// registerBackends auto-registers backend services declared in config.yaml
func registerBackends(cfg *config.Config, repo *repository.ApiToolRepo, grpcProxy *proxy.GrpcProxy, logger *zap.Logger) {
	if len(cfg.Backends) == 0 {
		return
	}
	logger.Info("auto-registering backends", zap.Int("count", len(cfg.Backends)))

	for _, b := range cfg.Backends {
		gwName := b.GatewayName
		if gwName == "" {
			gwName = "Default Gateway"
		}
		gw, err := repo.GetGatewayByName(gwName)
		if err != nil {
			gw = &model.Gateway{Name: gwName, Description: "auto-created from config"}
			repo.CreateGateway(gw)
		}

		if b.OpenAPIURL != "" || b.OpenAPISpec != "" {
			result, err := openapi.ParseSpec([]byte(b.OpenAPISpec), b.BaseURL)
			if err != nil {
				logger.Warn("openapi parse failed", zap.String("backend", b.Name), zap.Error(err))
				continue
			}
			tools := make([]model.ApiTool, 0, len(result.Tools))
			for _, t := range result.Tools {
				tools = append(tools, model.ApiTool{GatewayID: gw.ID, ToolName: t.ToolName, Description: t.Description, InputSchema: model.JSONMap(t.InputSchema), BackendUrl: t.BackendUrl, HttpMethod: t.HttpMethod, Protocol: "http"})
			}
			count, _ := repo.BatchCreate(tools)
			logger.Info("openapi tools imported", zap.Int("count", count), zap.String("backend", b.Name))
		}

		if b.GrpcProto != "" && b.GrpcAddr != "" {
			result, err := protobuf.ParseProto(b.GrpcProto, b.Name+".proto")
			if err != nil {
				logger.Warn("proto parse failed", zap.String("backend", b.Name), zap.Error(err))
				continue
			}
			if result.FDS != nil && len(result.Services) > 0 {
				for _, svc := range result.Services {
					grpcProxy.RegisterProto(svc, result.FDS)
				}
			}
			tools := make([]model.ApiTool, 0, len(result.Methods))
			for _, m := range result.Methods {
				tools = append(tools, model.ApiTool{GatewayID: gw.ID, ToolName: m.ToolName, Description: m.Description, InputSchema: model.JSONMap(m.InputSchema), BackendUrl: b.GrpcAddr, HttpMethod: m.MethodPath, Protocol: "grpc"})
			}
			count, _ := repo.BatchCreate(tools)
			logger.Info("grpc tools imported", zap.Int("count", count), zap.String("backend", b.Name))
		}
	}
}
