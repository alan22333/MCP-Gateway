// SSE 会话列表 handler
package handler

import (
	"github.com/gin-gonic/gin"
)

type SessionHandler struct {
	mgr *SessionManager
}

func NewSessionHandler(mgr *SessionManager) *SessionHandler {
	return &SessionHandler{mgr: mgr}
}

func (h *SessionHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/sessions", h.List)
}

func (h *SessionHandler) List(c *gin.Context) {
	sessions := h.mgr.List()
	c.JSON(200, gin.H{"sessions": sessions, "total": len(sessions)})
}
