package handler

import (
	"github.com/alan22333/mcp-nexus/internal/repository"

	"github.com/gin-gonic/gin"
)

// gatewayResolver 封装网关解析逻辑，被 SSE handler 和 Streamable HTTP handler 共用
type gatewayResolver struct {
	repo *repository.ApiToolRepo
}

// resolve 从请求参数中解析网关
// 优先级：api_key query param → gateway query param → 默认网关
func (r *gatewayResolver) resolve(c *gin.Context) (uint, string) {
	// 1. 从 api_key 查
	if apiKey := c.Query("api_key"); apiKey != "" {
		if key, err := r.repo.GetApiKeyByValue(apiKey); err == nil && key != nil {
			gw, gwErr := r.repo.GetGatewayByID(key.GatewayID)
			if gwErr == nil && gw.Enabled {
				return gw.ID, gw.Name
			}
		}
	}

	// 2. 从 gateway 参数查
	if gwName := c.Query("gateway"); gwName != "" {
		if gw, err := r.repo.GetGatewayByName(gwName); err == nil && gw.Enabled {
			return gw.ID, gw.Name
		}
	}

	// 3. 回退：默认网关
	if gw, err := r.repo.GetGatewayByName("Default Gateway"); err == nil {
		return gw.ID, gw.Name
	}

	return 0, "default"
}
