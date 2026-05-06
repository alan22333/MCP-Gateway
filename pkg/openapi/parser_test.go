package openapi

import (
	"testing"
)

const sampleSpec = `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
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

func TestParseSpec(t *testing.T) {
	tools, err := ParseSpec([]byte(sampleSpec), "http://api.example.com")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if len(tools) != 3 {
		t.Fatalf("期望解析出 3 个工具，得到 %d", len(tools))
	}

	// 转为 map 以便按名称查找（Go map 迭代顺序不确定）
	byName := make(map[string]ParsedTool)
	for _, tool := range tools {
		byName[tool.ToolName] = tool
	}

	// listOrders (GET /orders)
	listOrders, ok := byName["listOrders"]
	if !ok {
		t.Fatal("未找到 listOrders 工具")
	}
	if listOrders.HttpMethod != "GET" {
		t.Errorf("listOrders Method 期望 GET, 得到 %s", listOrders.HttpMethod)
	}
	if listOrders.BackendUrl != "http://api.example.com/orders" {
		t.Errorf("listOrders URL 不匹配: %s", listOrders.BackendUrl)
	}

	// createOrder (POST /orders/{id})
	createOrder, ok := byName["createOrder"]
	if !ok {
		t.Fatal("未找到 createOrder 工具")
	}
	if createOrder.HttpMethod != "POST" {
		t.Errorf("createOrder Method 期望 POST, 得到 %s", createOrder.HttpMethod)
	}

	// get_customers (GET /customers, 无 operationId 自动生成)
	getCustomers, ok := byName["get_customers"]
	if !ok {
		t.Fatal("未找到 get_customers 工具")
	}
	if getCustomers.HttpMethod != "GET" {
		t.Errorf("get_customers Method 期望 GET, 得到 %s", getCustomers.HttpMethod)
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

func TestParseSpecInvalidYAML(t *testing.T) {
	_, err := ParseSpec([]byte(`not: [valid yaml for openapi`), "http://x")
	if err == nil {
		t.Errorf("无效 YAML 应返回错误")
	}
}
