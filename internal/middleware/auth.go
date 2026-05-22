package middleware

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/alan22333/mcp-nexus/internal/repository"

	"github.com/gin-gonic/gin"
)

// AuthConfig 认证中间件启动配置（部分字段可通过 runtimeCfg 热更新）
type AuthConfig struct {
	Enabled     bool
	ExemptPaths []string
}

// RuntimeConfig 运行时可热更新的配置（通过 atomic.Value 在 main 和 middleware 间共享）
type RuntimeConfig struct {
	AuthEnabled bool
}

// APIKeyAuth 返回 API Key 认证中间件（per-gateway 判断）
// 1. 读 X-API-Key header 或 api_key query param
// 2. 查 ApiKey → 获取 GatewayID → 查 Gateway → 检查 APIKeyRequired
// 3. 无 key 时读 gateway query param → 查 Gateway → 检查 APIKeyRequired
// 4. 存储 gateway_id 到 gin context 供下游使用
// rtCfgPtr: 指向运行时配置的 atomic.Value，支持热更新 auth.enabled
func APIKeyAuth(cfg AuthConfig, repo *repository.ApiToolRepo, rtCfgPtr *atomic.Value) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查热更新后的 enabled 状态
		enabled := cfg.Enabled
		if rtCfgPtr != nil {
			if rt, ok := rtCfgPtr.Load().(*RuntimeConfig); ok {
				enabled = rt.AuthEnabled
			}
		}

		if !enabled {
			c.Next()
			return
		}

		// 豁免路径
		path := c.Request.URL.Path
		for _, exempt := range cfg.ExemptPaths {
			if exempt == "/" || exempt == "/index.html" {
				if path == exempt {
					c.Next()
					return
				}
			} else {
				if strings.HasPrefix(path, exempt) {
					c.Next()
					return
				}
			}
		}

		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		if apiKey != "" {
			// 有 key → 查 key → 查 gateway → 按 gateway.APIKeyRequired 判断
			key, err := repo.GetApiKeyByValue(apiKey)
			if err != nil || key == nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API Key 无效或已被禁用"})
				return
			}
			c.Set("api_key_name", key.Name)
			c.Set("gateway_id", key.GatewayID)

			// 即使 key 有效，如果 gateway 的 APIKeyRequired=false，也放行
			if gw, err := repo.GetGatewayByID(key.GatewayID); err == nil && !gw.APIKeyRequired {
				c.Next()
				return
			}
			c.Next()
			return
		}

		// 无 key → 查 gateway 参数
		gwName := c.Query("gateway")
		if gwName == "" {
			gwName = "Default Gateway"
		}
		gw, err := repo.GetGatewayByName(gwName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "网关不存在: " + gwName})
			return
		}

		c.Set("gateway_id", gw.ID)
		if gw.APIKeyRequired {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "此网关需要 API Key"})
			return
		}
		c.Next()
	}
}
