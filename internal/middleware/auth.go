package middleware

import (
	"net/http"
	"strings"

	"mcp-gateway-go-demo/internal/repository"

	"github.com/gin-gonic/gin"
)

// AuthConfig 认证中间件配置
type AuthConfig struct {
	Enabled     bool     // 是否启用认证（false 时直接放行）
	ExemptPaths []string // 豁免路径前缀列表（如 /metrics, /, /static）
}

// AuthKeyChecker 查询密钥是否合法的接口（repo 实现）
type AuthKeyChecker interface {
	GetApiKeyByValue(key string) (*struct{ Name string }, error)
}

// APIKeyAuth 返回 API Key 认证中间件
// 从 X-API-Key header 或 api_key query param 中读取密钥，查数据库验证
// 位于 ExemptPaths 中的路径不检查
func APIKeyAuth(cfg AuthConfig, repo *repository.ApiToolRepo) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 未启用 → 直接放行
		if !cfg.Enabled {
			c.Next()
			return
		}

		// 豁免路径 → 直接放行
		// "/" 和 "/index.html" 精确匹配；"/static/"、"/metrics" 等用前缀匹配
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

		// 从 Header 或 Query 获取 API Key
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "缺少 API Key，请在 X-API-Key header 或 api_key query 参数中提供",
			})
			return
		}

		// 查数据库验证
		key, err := repo.GetApiKeyByValue(apiKey)
		if err != nil || key == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "API Key 无效或已被禁用",
			})
			return
		}

		// 存入 context，供下游 handler 获取调用者身份
		c.Set("api_key_name", key.Name)
		c.Next()
	}
}
