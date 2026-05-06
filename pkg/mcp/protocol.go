// Package mcp 提供 MCP (Model Context Protocol) 基于 JSON-RPC 2.0 的基础协议封装。
// 参考规范: https://spec.modelcontextprotocol.io/
package mcp

import "encoding/json"

// JSONRPCVersion 是协议版本常量
const JSONRPCVersion = "2.0"

// RPCRequest 基础 JSON-RPC 2.0 请求结构体
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"` // 固定 "2.0"
	ID      interface{}     `json:"id"`      // 字符串或数字，用于关联请求与响应
	Method  string          `json:"method"`  // 方法名: "initialize", "tools/list", "tools/call"
	Params  json.RawMessage `json:"params"`  // 延迟解析的动态参数
}

// RPCResponse 基础 JSON-RPC 2.0 响应结构体
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError JSON-RPC 2.0 错误对象
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// 预定义的 JSON-RPC 2.0 标准错误码
const (
	ErrCodeParse     = -32700 // 解析错误
	ErrCodeInvalid   = -32600 // 无效请求
	ErrCodeMethod    = -32601 // 方法未找到
	ErrCodeInternal  = -32603 // 内部错误
)

// NewError 构造一个标准 JSON-RPC 错误响应
func NewError(id interface{}, code int, message string) *RPCResponse {
	return &RPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
}

// NewSuccess 构造一个标准 JSON-RPC 成功响应
func NewSuccess(id interface{}, result interface{}) *RPCResponse {
	return &RPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// Tool 表示 MCP 协议中的工具定义，在 tools/list 响应中返回给 AI
type Tool struct {
	Name        string      `json:"name"`        // 工具名称，如 "get_order_info"
	Description string      `json:"description"` // 工具功能描述，AI 依靠此字段判断何时使用
	InputSchema *JSONSchema `json:"inputSchema"` // JSON Schema 格式的参数定义
}

// JSONSchema 简化的 JSON Schema 结构，用于描述工具输入参数
type JSONSchema struct {
	Type       string                 `json:"type"`                 // 通常为 "object"
	Properties map[string]interface{} `json:"properties,omitempty"` // 参数属性定义
	Required   []string               `json:"required,omitempty"`   // 必填参数列表
}

// ToolsListResult 是 tools/list 方法的返回结果
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams 是 tools/call 方法的参数结构
type CallToolParams struct {
	Name      string          `json:"name"`      // 要调用的工具名称
	Arguments json.RawMessage `json:"arguments"` // AI 传入的参数 JSON
}

// CallToolResult 是 tools/call 方法的返回结果
type CallToolResult struct {
	Content []ContentItem `json:"content"` // 返回内容列表
}

// ContentItem 工具调用结果中的内容项
type ContentItem struct {
	Type string `json:"type"` // "text" 或 "resource"
	Text string `json:"text,omitempty"`
}
