package openapi

import (
	"testing"
)

const sampleOAS3 = `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: http://api.example.com
paths:
  /orders:
    get:
      operationId: listOrders
      description: 获取订单列表
      parameters:
        - name: status
          in: query
          schema:
            type: string
  /orders/{id}:
    post:
      operationId: createOrder
      description: 创建新订单
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                amount:
                  type: number
  /customers:
    get:
      summary: 获取客户列表
`

const sampleSwagger2 = `
swagger: "2.0"
info:
  title: Legacy API
  version: "1.0"
host: legacy.example.com
basePath: /api/v1
schemes:
  - https
paths:
  /users:
    get:
      operationId: listUsers
      description: 获取用户列表
      parameters:
        - name: role
          in: query
          type: string
    post:
      operationId: createUser
      description: 创建用户
`

func TestParseOpenAPI3(t *testing.T) {
	result, err := ParseSpec([]byte(sampleOAS3), "")
	if err != nil {
		t.Fatalf("解析 OpenAPI 3.0 失败: %v", err)
	}
	if result.SpecVersion != "openapi3" {
		t.Errorf("版本应为 openapi3, 得到 %s", result.SpecVersion)
	}
	if result.Title != "Test API" {
		t.Errorf("标题应为 Test API, 得到 %s", result.Title)
	}
	// servers auto-detect
	if result.BaseURL != "http://api.example.com" {
		t.Errorf("baseURL 应为 http://api.example.com, 得到 %s", result.BaseURL)
	}
	if len(result.Tools) != 3 {
		t.Fatalf("期望 3 个工具, 得到 %d", len(result.Tools))
	}

	byName := make(map[string]ParsedTool)
	for _, tl := range result.Tools {
		byName[tl.ToolName] = tl
	}
	if _, ok := byName["listOrders"]; !ok {
		t.Error("缺少 listOrders")
	}
	if _, ok := byName["createOrder"]; !ok {
		t.Error("缺少 createOrder")
	}
}

func TestParseSwagger2(t *testing.T) {
	result, err := ParseSpec([]byte(sampleSwagger2), "")
	if err != nil {
		t.Fatalf("解析 Swagger 2.0 失败: %v", err)
	}
	if result.SpecVersion != "swagger2" {
		t.Errorf("版本应为 swagger2, 得到 %s", result.SpecVersion)
	}
	// auto-detect baseURL from host+basePath+scheme
	if result.BaseURL != "https://legacy.example.com/api/v1" {
		t.Errorf("baseURL 应为 https://legacy.example.com/api/v1, 得到 %s", result.BaseURL)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("期望 2 个工具, 得到 %d", len(result.Tools))
	}

	byName := make(map[string]ParsedTool)
	for _, tl := range result.Tools {
		byName[tl.ToolName] = tl
	}
	if _, ok := byName["listUsers"]; !ok {
		t.Error("缺少 listUsers")
	}
	if _, ok := byName["createUser"]; !ok {
		t.Error("缺少 createUser")
	}
}

func TestParseSpecOverrideBaseURL(t *testing.T) {
	result, err := ParseSpec([]byte(sampleOAS3), "http://custom.example.com")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if result.BaseURL != "http://custom.example.com" {
		t.Errorf("手动指定的 baseURL 应覆盖 auto-detect: %s", result.BaseURL)
	}
}

func TestParseSpecInvalid(t *testing.T) {
	_, err := ParseSpec([]byte(`not a valid spec`), "")
	if err == nil {
		t.Error("无效文档应返回错误")
	}
}

func TestToMCPTools(t *testing.T) {
	parsed := []ParsedTool{
		{ToolName: "test", Description: "desc", InputSchema: nil, BackendUrl: "http://x", HttpMethod: "GET"},
	}
	mcpTools := ToMCPTools(parsed)
	if len(mcpTools) != 1 {
		t.Fatalf("期望 1 个 MCP Tool")
	}
	if mcpTools[0].Name != "test" {
		t.Errorf("名称不匹配")
	}
}
