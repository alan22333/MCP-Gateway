package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// parseUint 将字符串解析为 uint，解析失败返回 0
func parseUint(s string) uint {
	v, _ := strconv.ParseUint(s, 10, 64)
	return uint(v)
}

// parseGatewayID 从请求中提取 gateway_id（仅从 query param 获取，不读 body）
// 返回 0 表示"未指定"（调用方通常会 fallback 到全部网关）
func parseGatewayID(c *gin.Context) uint {
	if gwStr := c.Query("gateway_id"); gwStr != "" {
		return parseUint(gwStr)
	}
	return 0
}
