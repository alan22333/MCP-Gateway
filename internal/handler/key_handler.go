// API Key 管理 handlers：CRUD + toggle + 种子默认 key
package handler

import (
	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type KeyHandler struct {
	repo *repository.ApiToolRepo
}

func NewKeyHandler(repo *repository.ApiToolRepo) *KeyHandler {
	return &KeyHandler{repo: repo}
}

func (h *KeyHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/keys", h.List)
	r.POST("/api/keys", h.Create)
	r.DELETE("/api/keys/:id", h.Delete)
	r.PUT("/api/keys/:id/toggle", h.Toggle)
}

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
