// 网关管理 handlers：CRUD + toggle
package handler

import (
	"github.com/alan22333/mcp-nexus/internal/model"
	"github.com/alan22333/mcp-nexus/internal/repository"

	"github.com/gin-gonic/gin"
)

// GatewayHandler 网关管理 HTTP handler，提供网关的 CRUD 和启用/禁用操作
type GatewayHandler struct {
	repo *repository.ApiToolRepo
}

// NewGatewayHandler 创建网关管理 handler
func NewGatewayHandler(repo *repository.ApiToolRepo) *GatewayHandler {
	return &GatewayHandler{repo: repo}
}

// RegisterRoutes 注册网关管理路由到指定的 RouterGroup
func (h *GatewayHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/gateways", h.List)
	r.POST("/gateways", h.Create)
	r.DELETE("/gateways/:id", h.Delete)
	r.PUT("/gateways/:id/toggle", h.Toggle)
}

// List 返回所有网关列表
func (h *GatewayHandler) List(c *gin.Context) {
	gws, err := h.repo.ListGateways()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"gateways": gws, "total": len(gws)})
}

// Create 创建新网关，name 必填，api_key_required 决定该网关是否要求客户端提供 API Key
func (h *GatewayHandler) Create(c *gin.Context) {
	var input struct {
		Name           string `json:"name" binding:"required"`
		Description    string `json:"description"`
		APIKeyRequired bool   `json:"api_key_required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	gw := &model.Gateway{
		Name: input.Name, Description: input.Description, APIKeyRequired: input.APIKeyRequired,
	}
	if err := h.repo.CreateGateway(gw); err != nil {
		c.JSON(500, gin.H{"error": "创建网关失败: " + err.Error()})
		return
	}
	c.JSON(201, gw)
}

// Delete 删除指定 ID 的网关
func (h *GatewayHandler) Delete(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.JSON(400, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.repo.DeleteGateway(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "删除成功"})
}

// Toggle 切换网关的启用/禁用状态
func (h *GatewayHandler) Toggle(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.JSON(400, gin.H{"error": "无效的 ID"})
		return
	}
	gw, err := h.repo.ToggleGateway(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"id": gw.ID, "enabled": gw.Enabled})
}
