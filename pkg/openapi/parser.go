// Package openapi 提供 Swagger/OpenAPI 文档解析工具
// 支持 OpenAPI 3.0 和 Swagger 2.0 两种格式
package openapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"mcp-gateway-go-demo/pkg/mcp"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// normalizeYAML 将 YAML 文本转为 JSON，如果已经是 JSON 则原样返回
func normalizeYAML(data []byte) ([]byte, error) {
	// 快速检测：JSON 以 { 开头
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		return data, nil
	}
	// YAML → JSON
	var obj interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("YAML 解析失败: %w", err)
	}
	// yaml.v3 会把 map 解析为 map[string]interface{}，需要转 JSON 兼容格式
	normalized, err := json.Marshal(convertYAML(obj))
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// convertYAML 递归转换 yaml.v3 的输出为 JSON 兼容的 map（yaml.v3 用 map[string]interface{} 但 key 类型可能不同）
func convertYAML(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{})
		for k, v := range val {
			out[k] = convertYAML(v)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, v := range val {
			out[i] = convertYAML(v)
		}
		return out
	default:
		return val
	}
}

// ParsedTool 解析后的工具信息
type ParsedTool struct {
	ToolName    string                 `json:"tool_name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
	BackendUrl  string                 `json:"backend_url"`
	HttpMethod  string                 `json:"http_method"`
}

// ParseResult 解析结果，包含工具列表和检测到的 base URL
type ParseResult struct {
	Tools       []ParsedTool `json:"tools"`
	BaseURL     string       `json:"base_url"`     // 检测到的或用户指定的 base URL
	SpecVersion string       `json:"spec_version"` // "openapi3" 或 "swagger2"
	Title       string       `json:"title"`        // API 标题
}

// ParseSpec 自动检测格式（OpenAPI 3.0 / Swagger 2.0）并解析
// baseURL: 可选，为空时自动从 spec 的 servers/host 字段检测
func ParseSpec(specData []byte, baseURL string) (*ParseResult, error) {
	// 归一化：YAML → JSON（OpenAPI/Swagger 两种格式都可能以 YAML 提供）
	jsonData, err := normalizeYAML(specData)
	if err != nil {
		return nil, fmt.Errorf("文档格式错误: %w", err)
	}

	// 先尝试 OpenAPI 3.0
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(jsonData)
	if err == nil && doc.OpenAPI != "" {
		return parseOpenAPI3(doc, baseURL), nil
	}

	// 再尝试 Swagger 2.0（openapi2.T 用 JSON 反序列化）
	var doc2 openapi2.T
	if err2 := json.Unmarshal(jsonData, &doc2); err2 == nil && doc2.Swagger != "" {
		doc3, err3 := openapi2conv.ToV3(&doc2)
		if err3 != nil {
			return nil, fmt.Errorf("Swagger 2.0 转换为 OpenAPI 3.0 失败: %w", err3)
		}
		// Swagger 2.0 没有 servers，用 host + basePath 拼
		if baseURL == "" {
			scheme := "http"
			if len(doc2.Schemes) > 0 {
				scheme = doc2.Schemes[0]
			}
			baseURL = fmt.Sprintf("%s://%s%s", scheme, doc2.Host, doc2.BasePath)
			baseURL = strings.TrimSuffix(baseURL, "/")
		}
		result := parseOpenAPI3(doc3, baseURL)
		result.SpecVersion = "swagger2"
		return result, nil
	}

	return nil, fmt.Errorf("无法识别 API 文档格式（需要 OpenAPI 3.0 或 Swagger 2.0）")
}

// parseOpenAPI3 从已加载的 OpenAPI 3.0 文档解析工具列表
func parseOpenAPI3(doc *openapi3.T, baseURL string) *ParseResult {
	// auto-detect baseURL from servers
	if baseURL == "" && len(doc.Servers) > 0 {
		baseURL = strings.TrimSuffix(doc.Servers[0].URL, "/")
	}

	title := ""
	if doc.Info != nil {
		title = doc.Info.Title
	}

	var tools []ParsedTool
	for path, pathItem := range doc.Paths.Map() {
		operations := map[string]*openapi3.Operation{
			"GET": pathItem.Get, "POST": pathItem.Post,
			"PUT": pathItem.Put, "DELETE": pathItem.Delete,
		}
		for method, op := range operations {
			if op == nil {
				continue
			}
			tools = append(tools, ParsedTool{
				ToolName:    generateToolName(op.OperationID, method, path),
				Description: buildDescription(op),
				InputSchema: buildInputSchema(op),
				BackendUrl:  baseURL + path,
				HttpMethod:  method,
			})
		}
	}

	return &ParseResult{Tools: tools, BaseURL: baseURL, SpecVersion: "openapi3", Title: title}
}

func generateToolName(operationID, method, path string) string {
	if operationID != "" {
		return operationID
	}
	clean := strings.ReplaceAll(path, "/", "_")
	clean = strings.ReplaceAll(clean, "{", "")
	clean = strings.ReplaceAll(clean, "}", "")
	clean = strings.Trim(clean, "_")
	return strings.ToLower(method) + "_" + clean
}

func buildDescription(op *openapi3.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return op.OperationID
}

func buildInputSchema(op *openapi3.Operation) map[string]interface{} {
	schema := map[string]interface{}{}
	for _, param := range op.Parameters {
		if param.Value != nil && param.Value.Schema != nil && param.Value.Schema.Value != nil {
			schema[param.Value.Name] = map[string]interface{}{
				"type":        param.Value.Schema.Value.Type,
				"description": param.Value.Description,
			}
		}
	}
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		if media, ok := op.RequestBody.Value.Content["application/json"]; ok && media.Schema != nil && media.Schema.Value != nil {
			bodyBytes, _ := json.Marshal(media.Schema.Value)
			var bodySchema map[string]interface{}
			json.Unmarshal(bodyBytes, &bodySchema)
			if props, ok := bodySchema["properties"].(map[string]interface{}); ok {
				for k, v := range props {
					schema[k] = v
				}
			}
		}
	}
	return schema
}

// ToMCPTools 转换为 MCP Tool 格式
func ToMCPTools(parsed []ParsedTool) []mcp.Tool {
	tools := make([]mcp.Tool, 0, len(parsed))
	for _, p := range parsed {
		tools = append(tools, mcp.Tool{
			Name:        p.ToolName,
			Description: p.Description,
			InputSchema: &mcp.JSONSchema{Type: "object", Properties: p.InputSchema},
		})
	}
	return tools
}
