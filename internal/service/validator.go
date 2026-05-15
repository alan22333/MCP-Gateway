package service

import (
	"encoding/json"
	"fmt"

	"mcp-gateway-go-demo/internal/model"
)

// ValidationIssue 单个字段的校验问题（会返回给 AI，格式要 AI 能读懂）
type ValidationIssue struct {
	Field    string      `json:"field"`
	Expected string      `json:"expected"`
	Got      string      `json:"got"`
	Value    interface{} `json:"value,omitempty"`
	Missing  bool        `json:"missing,omitempty"` // required 字段缺失
}

// ValidateResult 校验结果
type ValidateResult struct {
	Valid  bool               `json:"-"`
	Issues []ValidationIssue  `json:"issues"`
}

// validateArgs 基于 ApiTool.InputSchema 校验 AI 传入的参数
// InputSchema 格式：{"field_name": {"type": "string", "description": "..."}, ...}
// 校验规则：
//   1. 参数类型必须匹配（string / number / boolean）
//   2. 标记为 required 的字段不能缺失
// 校验宽松：schema 未定义的额外字段不报错（AI 可能多传，后端自己处理）
func validateArgs(schema model.JSONMap, argsJSON json.RawMessage) *ValidateResult {
	result := &ValidateResult{Issues: []ValidationIssue{}}

	if len(schema) == 0 {
		result.Valid = true // 没有 schema 定义，不做校验
		return result
	}

	// 解析 AI 传入的参数
	var argsMap map[string]interface{}
	if len(argsJSON) > 0 {
		if err := json.Unmarshal(argsJSON, &argsMap); err != nil {
			result.Issues = append(result.Issues, ValidationIssue{
				Field:    "(arguments)",
				Expected: "合法 JSON",
				Got:      "解析失败",
				Value:    string(argsJSON),
			})
			return result
		}
	}

	// 遍历 schema 中定义的每个字段
	for fieldName, rawDef := range schema {
		def, ok := rawDef.(map[string]interface{})
		if !ok {
			continue
		}

		expectedType, _ := def["type"].(string)
		isRequired := false
		if req, ok := def["required"]; ok {
			if b, ok := req.(bool); ok {
				isRequired = b
			}
		}

		argVal, argExists := argsMap[fieldName]

		// 检查必填字段
		if isRequired && !argExists {
			result.Issues = append(result.Issues, ValidationIssue{
				Field:    fieldName,
				Expected: expectedType,
				Got:      "(缺失)",
				Missing:  true,
			})
			continue
		}

		// 如果参数不存在，跳过类型检查（可选字段）
		if !argExists {
			continue
		}

		// 检查类型
		if expectedType == "" {
			continue // schema 没声明 type，不校验
		}
		if !matchType(expectedType, argVal) {
			result.Issues = append(result.Issues, ValidationIssue{
				Field:    fieldName,
				Expected: expectedType,
				Got:      goTypeName(argVal),
				Value:    argVal,
			})
		}
	}

	result.Valid = len(result.Issues) == 0
	return result
}

// matchType 检查 Go 值的类型是否匹配 JSON Schema 声明的类型
func matchType(schemaType string, val interface{}) bool {
	switch schemaType {
	case "string":
		_, ok := val.(string)
		return ok
	case "number", "integer":
		// JSON 数字统一为 float64
		_, ok := val.(float64)
		return ok
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "array":
		_, ok := val.([]interface{})
		return ok
	case "object":
		_, ok := val.(map[string]interface{})
		return ok
	default:
		return true // 未知类型不校验
	}
}

// goTypeName 返回值的 Go 类型名（用于错误提示）
func goTypeName(val interface{}) string {
	switch val.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", val)
	}
}
