// SSE 会话列表 handler — 供管理后台查看当前活跃的 MCP 会话
package handler

import (
	"github.com/gin-gonic/gin"
)

// SessionHandler 会话列表 HTTP handler
type SessionHandler struct {
	mgr *SessionManager
}

// NewSessionHandler 创建会话列表 handler
func NewSessionHandler(mgr *SessionManager) *SessionHandler {
	return &SessionHandler{mgr: mgr}
}

// RegisterRoutes 注册会话管理路由
func (h *SessionHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/sessions", h.List)
}

// List 返回当前所有活跃会话（SSE 和 Streamable HTTP 均包括）
func (h *SessionHandler) List(c *gin.Context) {
	sessions := h.mgr.List()
	c.JSON(200, gin.H{"sessions": sessions, "total": len(sessions)})
}
