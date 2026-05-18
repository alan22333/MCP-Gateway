// 工具管理 handlers：CRUD + toggle + 同步测试
package handler

import (
	"encoding/json"

	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/internal/service"

	"github.com/gin-gonic/gin"
)

type ToolHandler struct {
	repo *repository.ApiToolRepo
	svc  *service.McpService
}

func NewToolHandler(repo *repository.ApiToolRepo, svc *service.McpService) *ToolHandler {
	return &ToolHandler{repo: repo, svc: svc}
}

func (h *ToolHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/tools", h.List)
	r.POST("/api/tools", h.Create)
	r.DELETE("/api/tools/:id", h.Delete)
	r.PUT("/api/tools/:id/toggle", h.Toggle)
	r.POST("/api/tools/test", h.Test)
}

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

func (h *ToolHandler) Create(c *gin.Context) {
	var input struct {
		GatewayID   uint                   `json:"gateway_id"`
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
	}
	if err := h.repo.Create(tool); err != nil {
		c.JSON(500, gin.H{"error": "创建工具失败: " + err.Error()})
		return
	}
	c.JSON(201, tool)
}

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

func (h *ToolHandler) Test(c *gin.Context) {
	var input struct {
		ToolName string          `json:"tool_name" binding:"required"`
		Args     json.RawMessage `json:"args"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}
	gatewayID := parseGatewayID(c)
	proxyResp, mcpResp := h.svc.CallTool(c.Request.Context(), gatewayID, input.ToolName, input.Args, "WEB")
	if mcpResp.Error != nil {
		c.JSON(502, gin.H{"error": mcpResp.Error.Message})
		return
	}
	var result interface{}
	if json.Unmarshal(proxyResp.Body, &result) != nil {
		result = string(proxyResp.Body)
	}
	c.JSON(200, gin.H{
		"status": proxyResp.StatusCode, "result": result, "mcp_response": mcpResp.Result,
	})
}
