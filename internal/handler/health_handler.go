// 后端健康检查 handler
package handler

import (
	"mcp-gateway-go-demo/internal/proxy"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	proxy *proxy.HttpProxy
}

func NewHealthHandler(p *proxy.HttpProxy) *HealthHandler {
	return &HealthHandler{proxy: p}
}

func (h *HealthHandler) RegisterRoutes(r gin.IRouter) {
	r.GET("/api/health", h.Check)
}

func (h *HealthHandler) Check(c *gin.Context) {
	_, err := h.proxy.Forward(c.Request.Context(), &proxy.ProxyRequest{
		Method: "GET", URL: "http://localhost:9090/",
	})
	if err != nil {
		c.JSON(200, gin.H{"backend": "offline", "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"backend": "online"})
}
