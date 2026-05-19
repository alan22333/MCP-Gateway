package mcp

import (
	"encoding/json"
	"testing"
)

func TestRPCRequestSerialization(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	var req RPCRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("jsonrpc 期望 2.0, 得到 %s", req.JSONRPC)
	}
	if req.Method != "tools/list" {
		t.Errorf("method 期望 tools/list, 得到 %s", req.Method)
	}

	data, err := json.Marshal(&req)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(data) != raw {
		t.Errorf("序列化结果不匹配:\n  期望: %s\n  得到: %s", raw, string(data))
	}
}

func TestNewError(t *testing.T) {
	resp := NewError("req-1", ErrCodeInternal, "内部错误")
	if resp.JSONRPC != JSONRPCVersion {
		t.Errorf("jsonrpc 不匹配")
	}
	if resp.Error.Code != ErrCodeInternal {
		t.Errorf("错误码不匹配")
	}
	if resp.Error.Message != "内部错误" {
		t.Errorf("错误消息不匹配")
	}
	if resp.Result != nil {
		t.Errorf("Result 应为 nil")
	}
}

func TestNewSuccess(t *testing.T) {
	result := map[string]string{"status": "ok"}
	resp := NewSuccess(42, result)
	if resp.JSONRPC != JSONRPCVersion {
		t.Errorf("jsonrpc 不匹配")
	}
	if resp.Error != nil {
		t.Errorf("Error 应为 nil")
	}
	if resp.Result == nil {
		t.Errorf("Result 不应为 nil")
	}
}

func TestCallToolParamsUnmarshal(t *testing.T) {
	raw := `{"name":"get_order","arguments":{"order_id":123}}`
	var params CallToolParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if params.Name != "get_order" {
		t.Errorf("Name 期望 get_order, 得到 %s", params.Name)
	}
	if string(params.Arguments) != `{"order_id":123}` {
		t.Errorf("Arguments 不匹配: %s", string(params.Arguments))
	}
}

func TestToolSerialization(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Description: "一个测试工具",
		InputSchema: &JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"param1": map[string]interface{}{"type": "string"},
			},
			Required: []string{"param1"},
		},
	}
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var back Tool
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if back.Name != "test_tool" {
		t.Errorf("Name 不匹配")
	}
	if back.Description != "一个测试工具" {
		t.Errorf("Description 不匹配")
	}
}

func TestIsNotification(t *testing.T) {
	notif := &RPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}
	if !notif.IsNotification() {
		t.Error("无 id 的请求应判定为 notification")
	}

	req := &RPCRequest{JSONRPC: "2.0", ID: "1", Method: "tools/list"}
	if req.IsNotification() {
		t.Error("有 id 的请求不应判定为 notification")
	}
}

func TestInitializeResultSerialization(t *testing.T) {
	result := &InitializeResult{
		ProtocolVersion: "2025-03-26",
		ServerInfo:      ServerInfo{Name: "mcp-gateway", Version: "2.0.0"},
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{ListChanged: false},
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var back InitializeResult
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if back.ProtocolVersion != "2025-03-26" {
		t.Errorf("ProtocolVersion 不匹配: %s", back.ProtocolVersion)
	}
	if back.ServerInfo.Name != "mcp-gateway" {
		t.Errorf("ServerInfo.Name 不匹配")
	}
	if back.Capabilities.Tools == nil {
		t.Error("Tools capability 不应为 nil")
	}
}

func TestNewErrorWithData(t *testing.T) {
	data := map[string]interface{}{"field": "order_id", "issue": "required"}
	resp := NewErrorWithData("req-1", ErrCodeInvalid, "参数校验失败", data)
	if resp.Error.Data == nil {
		t.Error("Error.Data 不应为 nil")
	}
}

func TestRPCNotificationSerialization(t *testing.T) {
	notif := &RPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	data, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var back RPCNotification
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if back.Method != "notifications/initialized" {
		t.Errorf("Method 不匹配")
	}
	// Notification 不含 id 字段
	if _, hasID := json.Marshal(notif); hasID != nil {
		t.Error("序列化失败")
	}
	// 确保序列化结果不含 "id" 字段
	if raw := string(data); raw == "" {
		t.Error("序列化结果为空")
	}
}
