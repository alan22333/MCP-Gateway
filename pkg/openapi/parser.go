// Package openapi 提供 Swagger/OpenAPI 文档解析工具
// 可将 OpenAPI 3.0 规范文档中的接口定义导入为 MCP Tool
package openapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"mcp-gateway-go-demo/pkg/mcp"

	"github.com/getkin/kin-openapi/openapi3"
)

// ParsedTool 解析后的工具信息，可直接存入数据库
type ParsedTool struct {
	ToolName    string                 // 工具名称
	Description string                 // 工具描述
	InputSchema map[string]interface{} // 参数 Schema
	BackendUrl  string                 // 推断的后端地址
	HttpMethod  string                 // HTTP 方法
}

// ParseSpec 解析 OpenAPI 3.0 文档，提取其中定义的操作作为工具列表
// baseURL: 后端服务的基础地址，用于拼装完整的 BackendUrl
func ParseSpec(specData []byte, baseURL string) ([]ParsedTool, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(specData)
	if err != nil {
		return nil, fmt.Errorf("OpenAPI 文档解析失败: %w", err)
	}

	var tools []ParsedTool

	for path, pathItem := range doc.Paths.Map() {
		// 遍历每个路径下的 HTTP 方法
		operations := map[string]*openapi3.Operation{
			"GET":    pathItem.Get,
			"POST":   pathItem.Post,
			"PUT":    pathItem.Put,
			"DELETE": pathItem.Delete,
		}

		for method, op := range operations {
			if op == nil {
				continue
			}

			tool := ParsedTool{
				ToolName:    generateToolName(op.OperationID, method, path),
				Description: buildDescription(op),
				InputSchema: buildInputSchema(op),
				BackendUrl:  strings.TrimRight(baseURL, "/") + path,
				HttpMethod:  method,
			}
			tools = append(tools, tool)
		}
	}

	return tools, nil
}

// generateToolName 生成工具名称：优先使用 operationId，否则用 method+path 拼接
func generateToolName(operationID, method, path string) string {
	if operationID != "" {
		return operationID
	}
	// 将路径中的 / 替换为 _，去掉首尾下划线
	clean := strings.ReplaceAll(path, "/", "_")
	clean = strings.ReplaceAll(clean, "{", "")
	clean = strings.ReplaceAll(clean, "}", "")
	clean = strings.Trim(clean, "_")
	return strings.ToLower(method) + "_" + clean
}

// buildDescription 构造工具描述
func buildDescription(op *openapi3.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return op.OperationID
}

// buildInputSchema 从 OpenAPI 参数和请求体构造 JSON Schema
func buildInputSchema(op *openapi3.Operation) map[string]interface{} {
	schema := map[string]interface{}{}

	// 提取路径参数和查询参数
	for _, param := range op.Parameters {
		if param.Value != nil {
			prop := map[string]interface{}{
				"type":        param.Value.Schema.Value.Type,
				"description": param.Value.Description,
			}
			schema[param.Value.Name] = prop
		}
	}

	// 提取 POST/PUT 的请求体 Schema
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		if media, ok := op.RequestBody.Value.Content["application/json"]; ok && media.Schema != nil {
			// 将整个请求体 Schema 序列化后合并
			bodyBytes, _ := json.Marshal(media.Schema.Value)
			var bodySchema map[string]interface{}
			json.Unmarshal(bodyBytes, &bodySchema)
			schema["body"] = bodySchema
		}
	}

	return schema
}

// ToMCPTools 将 ParsedTool 列表转换为 MCP Tool 列表
func ToMCPTools(parsed []ParsedTool) []mcp.Tool {
	tools := make([]mcp.Tool, 0, len(parsed))
	for _, p := range parsed {
		tools = append(tools, mcp.Tool{
			Name:        p.ToolName,
			Description: p.Description,
			InputSchema: &mcp.JSONSchema{
				Type:       "object",
				Properties: p.InputSchema,
			},
		})
	}
	return tools
}
