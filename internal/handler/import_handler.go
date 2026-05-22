// OpenAPI / Swagger 导入 handler
package handler

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/alan22333/mcp-nexus/internal/model"
	"github.com/alan22333/mcp-nexus/internal/repository"
	"github.com/alan22333/mcp-nexus/pkg/openapi"

	"github.com/gin-gonic/gin"
)

// ImportHandler OpenAPI/Swagger 文档导入 handler
type ImportHandler struct {
	repo *repository.ApiToolRepo
}

// NewImportHandler 创建导入 handler
func NewImportHandler(repo *repository.ApiToolRepo) *ImportHandler {
	return &ImportHandler{repo: repo}
}

// RegisterRoutes 注册导入路由
func (h *ImportHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/tools/import", h.Import)
}

// Import 处理 OpenAPI/Swagger 文档导入请求
// 支持两种来源：url（远程抓取）或 spec（直接传 JSON/YAML）
// 参数 base_url 可指定后端地址，不传则自动从 spec 的 servers 字段检测
// 参数 tool_names 可指定要导入的工具列表，不传则全量导入
// 参数 gateway_id 指定工具归属的网关，不传则从 query string 或默认值获取
// query: ?preview=true 只返回解析预览，不写入数据库
func (h *ImportHandler) Import(c *gin.Context) {
	var input struct {
		URL       string   `json:"url"`
		Spec      string   `json:"spec"`
		BaseURL   string   `json:"base_url"`
		ToolNames []string `json:"tool_names"`
		GatewayID uint     `json:"gateway_id"`
	}
	if input.GatewayID == 0 {
		input.GatewayID = parseGatewayID(c)
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}

	// 获取 spec 内容：优先 url，其次 spec 字段
	var specData []byte
	if input.URL != "" {
		data, err := httpGet(input.URL)
		if err != nil {
			c.JSON(400, gin.H{"error": "获取 API 文档失败: " + err.Error()})
			return
		}
		specData = data
	} else if input.Spec != "" {
		specData = []byte(input.Spec)
	} else {
		c.JSON(400, gin.H{"error": "请提供 url 或 spec"})
		return
	}

	result, err := openapi.ParseSpec(specData, input.BaseURL)
	if err != nil {
		c.JSON(400, gin.H{"error": "API 文档解析失败: " + err.Error()})
		return
	}

	// 预览模式：只返回解析结果，不写入数据库
	if c.Query("preview") == "true" {
		c.JSON(200, gin.H{
			"preview": true, "title": result.Title, "base_url": result.BaseURL,
			"spec_version": result.SpecVersion, "tools": result.Tools, "total": len(result.Tools),
		})
		return
	}

	// 按 tool_names 过滤
	nameSet := make(map[string]bool)
	for _, n := range input.ToolNames {
		nameSet[n] = true
	}
	hasFilter := len(input.ToolNames) > 0

	tools := make([]model.ApiTool, 0, len(result.Tools))
	skipped := 0
	for _, p := range result.Tools {
		if hasFilter && !nameSet[p.ToolName] {
			skipped++
			continue
		}
		tools = append(tools, model.ApiTool{
			GatewayID: input.GatewayID, ToolName: p.ToolName, Description: p.Description,
			InputSchema: p.InputSchema, BackendUrl: p.BackendUrl, HttpMethod: p.HttpMethod,
		})
	}

	count, err := h.repo.BatchCreate(tools)
	if err != nil {
		c.JSON(500, gin.H{"error": "批量创建失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"message":  fmt.Sprintf("成功导入 %d/%d 个工具", count, len(tools)),
		"total":    len(result.Tools),
		"created":  count,
		"skipped":  skipped,
		"base_url": result.BaseURL,
		"title":    result.Title,
	})
}

// httpGet 发起 HTTP GET 请求，10s 超时，返回响应 body
func httpGet(rawURL string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
