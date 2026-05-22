package main

import (
	"mcp-gateway-go-demo/internal/handler"
	"mcp-gateway-go-demo/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// registerRoutes 注册所有路由分组，返回配置好的 gin.Engine
func registerRoutes(deps *dependencies) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 全局中间件
	r.Use(middleware.TraceID())
	r.Use(middleware.BodyLimit(int64(deps.cfg.Server.MaxBodyBytes)))

	// ── Group 1: Observability & Health (no auth) ──
	publicGroup := r.Group("")
	{
		publicGroup.GET("/metrics", gin.WrapH(promhttp.Handler()))
		publicGroup.GET("/api/health", handler.NewHealthHandler().Check)
	}

	// ── Group 2: MCP protocol endpoints (auth middleware) ──
	mcpGroup := r.Group("/mcp", middleware.APIKeyAuth(middleware.AuthConfig{
		Enabled: deps.cfg.Auth.Enabled, ExemptPaths: deps.cfg.Auth.ExemptPaths,
	}, deps.apiRepo, &deps.rtCfgPtr))

	if _, err := deps.apiRepo.EnsureDefaultGateway(); err != nil {
		deps.logger.Warn("default gateway migration failed", zap.Error(err))
	}

	// Old SSE transport (backward compatible)
	mcpGroup.GET("/sse", deps.mcpHandler.HandleSSE)          // 建立连接（session）
	mcpGroup.POST("/message", deps.mcpHandler.HandleMessage) // 发送MCP信息（JSON-RPC）

	// New Streamable HTTP transport (MCP 2025 spec)
	mcpGroup.POST("", deps.streamHandler.Handle)

	// ── Group 3: Admin API (auth middleware) ──
	//
	// 【知识点：路由注册方式的对比 (直接注册 vs 委托注册)】
	// 上方的 Group 1 & 2 使用的是“直接注册”，即在 routes.go 中直接写明路径和 Handler，如 mcpGroup.GET("/sse", ...)。
	// 这种模式适合路径单一明确、功能独立的接口，一目了然。
	//
	// 下方的 Admin API 组使用的是“委托注册（模块化）”。因为后台管理包含大量复杂的 RESTful 接口
	// (各种 CRUD：GET /tools, POST /tools, PUT /tools/:id 等)。为了防止本文件变得冗长臃肿，
	// 此处仅创建顶层的路由组 (apiGroup)，并将其传递给各个具体的 Handler（如 NewGatewayHandler）。
	// 各子模块在其内部的 RegisterRoutes 方法中自行规划和管理细分路径。这种做法实现了“高内聚低耦合”。
	apiGroup := r.Group("/api", middleware.APIKeyAuth(middleware.AuthConfig{
		Enabled: deps.cfg.Auth.Enabled, ExemptPaths: deps.cfg.Auth.ExemptPaths,
	}, deps.apiRepo, &deps.rtCfgPtr))

	// 网关配置管理路由：处理多租户/网关实例等信息的查询、创建和更新
	handler.NewGatewayHandler(deps.apiRepo).RegisterRoutes(apiGroup)
	// 工具管理路由：处理 MCP Tool（工具）的增删改查、启用禁用及测试等
	handler.NewToolHandler(deps.apiRepo, deps.svc).RegisterRoutes(apiGroup)
	// OpenAPI 导入路由：用于从 Swagger/OpenAPI 规范规范中一键导入 REST API 作为 MCP Tools
	handler.NewImportHandler(deps.apiRepo).RegisterRoutes(apiGroup)
	// gRPC 导入路由：用于解析 .proto 文件并导入 gRPC 服务作为 MCP Tools
	handler.NewGrpcImportHandler(deps.apiRepo, deps.grpcProxy).RegisterRoutes(apiGroup)
	// 日志管理路由：提供对 MCP 工具调用历史、流转日志的查询接口
	handler.NewLogHandler(deps.apiRepo).RegisterRoutes(apiGroup)
	// 会话管理路由：管理当前通过 SSE 和 Streamable HTTP 连接的激活状态（Sessions）
	handler.NewSessionHandler(deps.sessionMgr).RegisterRoutes(apiGroup)

	keyHandler := handler.NewKeyHandler(deps.apiRepo)
	keyHandler.RegisterRoutes(apiGroup)
	if deps.cfg.Auth.Enabled {
		keyHandler.SeedDefault(deps.logger)
	}

	// ── Group 4: Admin UI (no auth) ──
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/index.html", "./web/index.html")
	r.Static("/static", "./web/static")

	return r
}
