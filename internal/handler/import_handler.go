// OpenAPI / Swagger 导入 handler
package handler

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/pkg/openapi"

	"github.com/gin-gonic/gin"
)

// ImportHandler OpenAPI 导入 handler
type ImportHandler struct {
	repo *repository.ApiToolRepo
}

func NewImportHandler(repo *repository.ApiToolRepo) *ImportHandler {
	return &ImportHandler{repo: repo}
}

func (h *ImportHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/api/tools/import", h.Import)
}

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

	if c.Query("preview") == "true" {
		c.JSON(200, gin.H{
			"preview": true, "title": result.Title, "base_url": result.BaseURL,
			"spec_version": result.SpecVersion, "tools": result.Tools, "total": len(result.Tools),
		})
		return
	}

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
