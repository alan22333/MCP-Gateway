// 调用日志 handler
package handler

import (
	"strconv"

	"mcp-gateway-go-demo/internal/repository"

	"github.com/gin-gonic/gin"
)

type LogHandler struct {
	repo *repository.ApiToolRepo
}

func NewLogHandler(repo *repository.ApiToolRepo) *LogHandler {
	return &LogHandler{repo: repo}
}

func (h *LogHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/logs", h.List)
}

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
