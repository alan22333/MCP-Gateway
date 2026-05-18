package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func parseUint(s string) uint {
	v, _ := strconv.ParseUint(s, 10, 64)
	return uint(v)
}

// parseGatewayID 从请求中提取 gateway_id（query param > JSON body > 默认 0）
func parseGatewayID(c *gin.Context) uint {
	if gwStr := c.Query("gateway_id"); gwStr != "" {
		return parseUint(gwStr)
	}
	return 0
}
