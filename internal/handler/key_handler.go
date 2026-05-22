// API Key 管理 handlers：CRUD + toggle + 种子默认 key
package handler

import (
	"github.com/alan22333/mcp-nexus/internal/model"
	"github.com/alan22333/mcp-nexus/internal/repository"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// KeyHandler API Key 管理 HTTP handler
type KeyHandler struct {
	repo *repository.ApiToolRepo
}

// NewKeyHandler 创建 API Key 管理 handler
func NewKeyHandler(repo *repository.ApiToolRepo) *KeyHandler {
	return &KeyHandler{repo: repo}
}

// RegisterRoutes 注册 API Key 管理路由到指定的 RouterGroup
func (h *KeyHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/keys", h.List)
	r.POST("/keys", h.Create)
	r.DELETE("/keys/:id", h.Delete)
	r.PUT("/keys/:id/toggle", h.Toggle)
}

// SeedDefault 在认证启用时确保存在一条默认演示密钥
// 如果密钥已存在则跳过，避免重复创建
func (h *KeyHandler) SeedDefault(logger *zap.Logger) {
	defaultKey := "mcp-gw-sk-demo-key-2026"
	_, err := h.repo.GetApiKeyByValue(defaultKey)
	if err == nil {
		return
	}
	if err := h.repo.CreateApiKey(&model.ApiKey{
		Key: defaultKey, Name: "默认演示密钥",
	}); err != nil {
		logger.Warn("创建默认 API Key 失败", zap.Error(err))
	} else {
		logger.Info("已创建默认 API Key", zap.String("key", defaultKey))
	}
}

// List 返回 API Key 列表，可通过 ?gateway_id=X 过滤
func (h *KeyHandler) List(c *gin.Context) {
	gatewayID := parseGatewayID(c)
	var keys []model.ApiKey
	var err error
	if gatewayID > 0 {
		keys, err = h.repo.GetApiKeysByGateway(gatewayID)
	} else {
		keys, err = h.repo.ListApiKeys()
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"keys": keys, "total": len(keys)})
}

// Create 创建新 API Key，gateway_id 可选
// key 字段在数据库中为唯一索引，重复创建会返回 500
func (h *KeyHandler) Create(c *gin.Context) {
	var input struct {
		GatewayID uint   `json:"gateway_id"`
		Key       string `json:"key" binding:"required"`
		Name      string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "参数校验失败: " + err.Error()})
		return
	}
	if input.GatewayID == 0 {
		input.GatewayID = parseGatewayID(c)
	}
	ak := &model.ApiKey{GatewayID: input.GatewayID, Key: input.Key, Name: input.Name}
	if err := h.repo.CreateApiKey(ak); err != nil {
		c.JSON(500, gin.H{"error": "创建 API Key 失败: " + err.Error()})
		return
	}
	c.JSON(201, ak)
}

// Delete 删除指定 ID 的 API Key
func (h *KeyHandler) Delete(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.JSON(400, gin.H{"error": "无效的 ID"})
		return
	}
	if err := h.repo.DeleteApiKey(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "删除成功"})
}

// Toggle 切换 API Key 的启用/禁用状态
func (h *KeyHandler) Toggle(c *gin.Context) {
	id := parseUint(c.Param("id"))
	if id == 0 {
		c.JSON(400, gin.H{"error": "无效的 ID"})
		return
	}
	key, err := h.repo.ToggleApiKey(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"id": key.ID, "enabled": key.Enabled})
}
