// 调用日志 handler — 供管理后台查看 tools/call 的历史记录
package handler

import (
	"strconv"

	"mcp-gateway-go-demo/internal/repository"

	"github.com/gin-gonic/gin"
)

// LogHandler 调用日志 HTTP handler
type LogHandler struct {
	repo *repository.ApiToolRepo
}

// NewLogHandler 创建日志查询 handler
func NewLogHandler(repo *repository.ApiToolRepo) *LogHandler {
	return &LogHandler{repo: repo}
}

// RegisterRoutes 注册日志路由
func (h *LogHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/logs", h.List)
}

// List 返回最近的调用日志，默认 50 条，上限 200 条
func (h *LogHandler) List(c *gin.Context) {
	limit := 50
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	logs, err := h.repo.GetCallLogs(limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"logs": logs, "total": len(logs)})
}
