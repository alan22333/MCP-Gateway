// 工具管理 handlers：CRUD + toggle + 同步测试
package handler

import (
	"encoding/json"

	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/internal/service"

	"github.com/gin-gonic/gin"
)

// ToolHandler 工具管理 HTTP handler，提供 ApiTool 的 CRUD、Toggle 和同步测试
type ToolHandler struct {
	repo *repository.ApiToolRepo
	svc  *service.McpService // 用于同步测试工具调用
}

// NewToolHandler 创建工具管理 handler
func NewToolHandler(repo *repository.ApiToolRepo, svc *service.McpService) *ToolHandler {
	return &ToolHandler{repo: repo, svc: svc}
}

// RegisterRoutes 注册工具管理路由到指定的 RouterGroup
func (h *ToolHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/tools", h.List)
	r.POST("/tools", h.Create)
	r.DELETE("/tools/:id", h.Delete)
	r.PUT("/tools/:id/toggle", h.Toggle)
	r.POST("/tools/test", h.Test)
}

// List 返回工具列表，可通过 ?gateway_id=X 过滤
func (h *ToolHandler) List(c *gin.Context) {
	gatewayID := parseGatewayID(c)
	var tools []model.ApiTool
	var err error
	if gatewayID > 0 {
		tools, err = h.repo.GetToolsByGateway(gatewayID)
	} else {
		tools, err = h.repo.GetAllTools()
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, tools)
}

// Create 创建新工具，HttpMethod 仅支持 GET/POST
// gateway_id 可选，不传则从 query string 或默认值获取
func (h *ToolHandler) Create(c *gin.Context) {
	var input struct {
		GatewayID   uint                   `json:"gateway_id"`
		ToolName    string                 `json:"tool_name" binding:"required"`
		Description string                 `json:"description" binding:"required"`
		InputSchema map[string]interface{} `json:"input_schema"`
		BackendUrl  string                 `json:"backend_url" binding:"required"`
		HttpMethod  string                 `json:"http_method"`
		Protocol    string                 `json:"protocol"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}
	if input.HttpMethod == "" {
		input.HttpMethod = "POST"
	}
	if input.Protocol == "" {
		input.Protocol = "http"
	}
	if input.Protocol == "http" && input.HttpMethod != "GET" && input.HttpMethod != "POST" {
		c.JSON(400, gin.H{"error": "HTTP 协议只支持 GET 或 POST"})
		return
	}
	if input.GatewayID == 0 {
		input.GatewayID = parseGatewayID(c)
	}
	tool := &model.ApiTool{
		GatewayID:   input.GatewayID,
		ToolName:    input.ToolName,
		Description: input.Description,
		InputSchema: input.InputSchema,
		BackendUrl:  input.BackendUrl,
		HttpMethod:  input.HttpMethod,
		Protocol:    input.Protocol,
	}
	if err := h.repo.Create(tool); err != nil {
		c.JSON(500, gin.H{"error": "创建工具失败: " + err.Error()})
		return
	}
	c.JSON(201, tool)
}

// Delete 删除指定 ID 的工具
func (h *ToolHandler) Delete(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.JSON(400, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.repo.Delete(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "删除成功"})
}

// Toggle 切换工具的启用/禁用状态
func (h *ToolHandler) Toggle(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.JSON(400, gin.H{"error": "无效的 ID"})
		return
	}
	tool, err := h.repo.ToggleEnabled(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"id": tool.ID, "enabled": tool.Enabled})
}

// Test 同步测试工具调用：直接调用 McpService.CallTool 并返回代理响应
// 用于管理后台在不通过 SSE/Streamable 的情况下快速验证工具配置
func (h *ToolHandler) Test(c *gin.Context) {
	var input struct {
		ToolName  string          `json:"tool_name" binding:"required"`
		Args      json.RawMessage `json:"args"`
		GatewayID uint            `json:"gateway_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}
	gatewayID := input.GatewayID
	if gatewayID == 0 {
		gatewayID = parseGatewayID(c)
	}
	proxyResp, mcpResp := h.svc.CallTool(c.Request.Context(), gatewayID, input.ToolName, input.Args, "WEB")
	if mcpResp.Error != nil {
		c.JSON(502, gin.H{"error": mcpResp.Error.Message})
		return
	}
	var result interface{}
	if err := json.Unmarshal(proxyResp.Body, &result); err != nil {
		result = string(proxyResp.Body)
	}
	c.JSON(200, gin.H{
		"status": proxyResp.StatusCode, "result": result, "mcp_response": mcpResp.Result,
	})
}
