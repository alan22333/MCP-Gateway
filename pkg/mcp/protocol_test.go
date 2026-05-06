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
