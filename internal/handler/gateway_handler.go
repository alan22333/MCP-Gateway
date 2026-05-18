// 网关管理 handlers：CRUD + toggle
package handler

import (
	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"

	"github.com/gin-gonic/gin"
)

type GatewayHandler struct {
	repo *repository.ApiToolRepo
}

func NewGatewayHandler(repo *repository.ApiToolRepo) *GatewayHandler {
	return &GatewayHandler{repo: repo}
}

func (h *GatewayHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/gateways", h.List)
	r.POST("/api/gateways", h.Create)
	r.DELETE("/api/gateways/:id", h.Delete)
	r.PUT("/api/gateways/:id/toggle", h.Toggle)
}

func (h *GatewayHandler) List(c *gin.Context) {
	gws, err := h.repo.ListGateways()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"gateways": gws, "total": len(gws)})
}

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
