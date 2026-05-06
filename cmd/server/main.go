// MCP Gateway 服务入口
// 启动 Gin HTTP 服务器，建立 SSE 长连接通道，提供 MCP 协议转换与后端代理能力
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"mcp-gateway-go-demo/internal/config"
	"mcp-gateway-go-demo/internal/handler"
	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/proxy"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/internal/service"
	"mcp-gateway-go-demo/pkg/openapi"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

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

	httpProxy := proxy.NewHttpProxy()
	svc := service.NewMcpService(apiRepo, httpProxy, logger)
	sessionMgr := handler.NewSessionManager()
	mcpHandler := handler.NewMcpHandler(sessionMgr, svc, logger)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// MCP 协议端点
	r.GET("/mcp/sse", mcpHandler.HandleSSE)
	r.POST("/mcp/message", mcpHandler.HandleMessage)

	// 工具管理 API
	r.GET("/api/tools", handleListTools(apiRepo))
	r.POST("/api/tools", handleCreateTool(apiRepo))
	r.DELETE("/api/tools/:id", handleDeleteTool(apiRepo))
	r.PUT("/api/tools/:id/toggle", handleToggleTool(apiRepo))

	// OpenAPI 批量导入
	r.POST("/api/tools/import", handleImportOpenAPI(apiRepo))

	// 调用日志 API
	r.GET("/api/logs", handleListLogs(apiRepo))

	// 会话管理 API
	r.GET("/api/sessions", handleListSessions(sessionMgr))

	// 同步工具测试（通过 service 走，带日志）
	r.POST("/api/tools/test", handleTestTool(svc))

	// 后端健康检查
	r.GET("/api/health", handleHealth(httpProxy))

	// 管理后台前端
	r.StaticFile("/", "./web/index.html")
	r.StaticFile("/index.html", "./web/index.html")
	r.Static("/static", "./web/static")

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	logger.Info("MCP Gateway 启动",
		zap.String("address", addr),
		zap.String("dashboard", "http://localhost"+addr),
	)
	if err := r.Run(addr); err != nil {
		logger.Fatal("服务启动失败", zap.Error(err))
	}
}

// ---------- 工具管理 ----------

func handleListTools(repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		tools, err := repo.GetAll()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, tools)
	}
}

func handleCreateTool(repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			ToolName    string                 `json:"tool_name" binding:"required"`
			Description string                 `json:"description" binding:"required"`
			InputSchema map[string]interface{} `json:"input_schema"`
			BackendUrl  string                 `json:"backend_url" binding:"required"`
			HttpMethod  string                 `json:"http_method" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "参数校验失败: " + err.Error()})
			return
		}
		if input.HttpMethod != "GET" && input.HttpMethod != "POST" {
			c.JSON(400, gin.H{"error": "HttpMethod 只支持 GET 或 POST"})
			return
		}

		apiTool := &model.ApiTool{
			ToolName:    input.ToolName,
			Description: input.Description,
			InputSchema: input.InputSchema,
			BackendUrl:  input.BackendUrl,
			HttpMethod:  input.HttpMethod,
		}
		if err := repo.Create(apiTool); err != nil {
			c.JSON(500, gin.H{"error": "创建工具失败: " + err.Error()})
			return
		}
		c.JSON(201, apiTool)
	}
}

func handleDeleteTool(repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := parseUint(c.Param("id"))
		if id == 0 {
			c.JSON(400, gin.H{"error": "无效的 ID"})
			return
		}
		if err := repo.Delete(id); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"message": "删除成功"})
	}
}

func handleToggleTool(repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := parseUint(c.Param("id"))
		if id == 0 {
			c.JSON(400, gin.H{"error": "无效的 ID"})
			return
		}
		tool, err := repo.ToggleEnabled(id)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"id": tool.ID, "enabled": tool.Enabled})
	}
}

// ---------- OpenAPI 批量导入 ----------

func handleImportOpenAPI(repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		baseURL := c.Query("base_url")
		if baseURL == "" {
			baseURL = "http://localhost:9090"
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(400, gin.H{"error": "读取请求体失败"})
			return
		}

		parsed, err := openapi.ParseSpec(body, baseURL)
		if err != nil {
			c.JSON(400, gin.H{"error": "OpenAPI 解析失败: " + err.Error()})
			return
		}

		tools := make([]model.ApiTool, 0, len(parsed))
		for _, p := range parsed {
			tools = append(tools, model.ApiTool{
				ToolName:    p.ToolName,
				Description: p.Description,
				InputSchema: p.InputSchema,
				BackendUrl:  p.BackendUrl,
				HttpMethod:  p.HttpMethod,
			})
		}

		count, err := repo.BatchCreate(tools)
		if err != nil {
			c.JSON(500, gin.H{"error": "批量创建失败: " + err.Error()})
			return
		}

		c.JSON(200, gin.H{
			"message": fmt.Sprintf("成功导入 %d/%d 个工具", count, len(tools)),
			"total":   len(tools),
			"created": count,
		})
	}
}

// ---------- 调用日志 ----------

func handleListLogs(repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 50
		if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && l > 0 && l <= 200 {
			limit = l
		}
		logs, err := repo.GetCallLogs(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"logs": logs, "total": len(logs)})
	}
}

// ---------- 会话管理 ----------

func handleListSessions(mgr *handler.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessions := mgr.List()
		c.JSON(200, gin.H{"sessions": sessions, "total": len(sessions)})
	}
}

// ---------- 健康检查 ----------

func handleHealth(p *proxy.HttpProxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, err := p.Forward(&proxy.ProxyRequest{
			Method: "GET",
			URL:    "http://localhost:9090/",
		})
		if err != nil {
			c.JSON(200, gin.H{"backend": "offline", "error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"backend": "online"})
	}
}

// ---------- 同步工具测试（走 service.CallTool，带日志记录）----------

func handleTestTool(svc *service.McpService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			ToolName string          `json:"tool_name" binding:"required"`
			Args     json.RawMessage `json:"args"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": "参数校验失败: " + err.Error()})
			return
		}

		proxyResp, mcpResp := svc.CallTool(input.ToolName, input.Args, "WEB")
		if mcpResp.Error != nil {
			c.JSON(502, gin.H{"error": mcpResp.Error.Message})
			return
		}

		var result interface{}
		if err := json.Unmarshal(proxyResp.Body, &result); err != nil {
			result = string(proxyResp.Body)
		}

		c.JSON(200, gin.H{
			"status":       proxyResp.StatusCode,
			"result":       result,
			"mcp_response": mcpResp.Result,
		})
	}
}

func parseUint(s string) uint {
	v, _ := strconv.ParseUint(s, 10, 64)
	return uint(v)
}
