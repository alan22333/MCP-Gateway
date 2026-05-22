// 健康检查 handler — 返回网关自身运行状态
package handler

import (
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查 handler
type HealthHandler struct{}

// NewHealthHandler 创建健康检查 handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Check 返回网关自身健康状态
func (h *HealthHandler) Check(c *gin.Context) {
	c.JSON(200, gin.H{
		"gateway": "online",
		"version": "2.2.0",
	})
}
