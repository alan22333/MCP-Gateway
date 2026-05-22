// 种子数据工具 —— 创建演示网关和工具
// 配合 mock-backend 使用，用于本地端到端测试
//
// 使用方式:
//
//	go run cmd/seed/main.go
package main

import (
	"log"

	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	db, err := gorm.Open(sqlite.Open("gateway.db"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	repo := repository.NewApiToolRepo(db)
	if err := repo.AutoMigrate(); err != nil {
		log.Fatalf("自动建表失败: %v", err)
	}

	// ── 创建三个网关 ──
	gateways := []model.Gateway{
		{Name: "Default Gateway", Description: "系统默认网关（所有工具）", APIKeyRequired: false},
		{Name: "订单服务", Description: "订单查询、创建、详情", APIKeyRequired: true},
		{Name: "客户与库存", Description: "客户查询 + 库存管理", APIKeyRequired: false},
	}
	var gwIDs [3]uint
	for i := range gateways {
		if err := repo.CreateGateway(&gateways[i]); err != nil {
			log.Printf("创建网关 '%s' 失败（可能已存在）: %v", gateways[i].Name, err)
			// 尝试查找已存在的
			if g, e := repo.GetGatewayByName(gateways[i].Name); e == nil {
				gwIDs[i] = g.ID
				log.Printf("  → 使用已有网关 ID=%d", g.ID)
			}
		} else {
			gwIDs[i] = gateways[i].ID
			log.Printf("✓ 创建网关: %s (ID=%d, APIKey=%v)", gateways[i].Name, gateways[i].ID, gateways[i].APIKeyRequired)
		}
	}

	defGW := gwIDs[0]
	orderGW := gwIDs[1]
	crmGW := gwIDs[2]

	// ── Default Gateway: 7 个工具（全集）──
	tools := []model.ApiTool{
		{GatewayID: defGW, ToolName: "query_orders", Description: "查询订单列表。可按客户ID(customer)和订单状态(status)筛选", InputSchema: map[string]interface{}{"customer": map[string]interface{}{"type": "string"}, "status": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/orders", HttpMethod: "GET"},
		{GatewayID: defGW, ToolName: "get_order_detail", Description: "根据订单ID查询单个订单详情", InputSchema: map[string]interface{}{"id": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/orders/{id}", HttpMethod: "GET"},
		{GatewayID: defGW, ToolName: "create_order", Description: "创建新订单，需提供客户ID和金额", InputSchema: map[string]interface{}{"customer": map[string]interface{}{"type": "string"}, "amount": map[string]interface{}{"type": "number"}}, BackendUrl: "http://localhost:9090/api/orders", HttpMethod: "POST"},
		{GatewayID: defGW, ToolName: "query_customers", Description: "查询客户列表，可按等级(level)筛选", InputSchema: map[string]interface{}{"level": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/customers", HttpMethod: "GET"},
		{GatewayID: defGW, ToolName: "get_customer_detail", Description: "根据客户ID查询客户详情", InputSchema: map[string]interface{}{"id": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/customers/{id}", HttpMethod: "GET"},
		{GatewayID: defGW, ToolName: "query_inventory", Description: "查询库存信息，可按仓库筛选", InputSchema: map[string]interface{}{"warehouse": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/inventory", HttpMethod: "GET"},
		{GatewayID: defGW, ToolName: "get_inventory_item", Description: "根据SKU查询商品库存详情", InputSchema: map[string]interface{}{"sku": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/inventory/{sku}", HttpMethod: "GET"},

		// ── 订单服务网关: 3 个订单工具 ──
		{GatewayID: orderGW, ToolName: "query_orders", Description: "查询订单列表。可按客户ID(customer)和订单状态(status)筛选", InputSchema: map[string]interface{}{"customer": map[string]interface{}{"type": "string"}, "status": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/orders", HttpMethod: "GET"},
		{GatewayID: orderGW, ToolName: "get_order_detail", Description: "根据订单ID查询单个订单详情", InputSchema: map[string]interface{}{"id": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/orders/{id}", HttpMethod: "GET"},
		{GatewayID: orderGW, ToolName: "create_order", Description: "创建新订单，需提供客户ID和金额", InputSchema: map[string]interface{}{"customer": map[string]interface{}{"type": "string"}, "amount": map[string]interface{}{"type": "number"}}, BackendUrl: "http://localhost:9090/api/orders", HttpMethod: "POST"},

		// ── 客户与库存网关: 4 个工具 ──
		{GatewayID: crmGW, ToolName: "query_customers", Description: "查询客户列表，可按等级(level)筛选", InputSchema: map[string]interface{}{"level": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/customers", HttpMethod: "GET"},
		{GatewayID: crmGW, ToolName: "get_customer_detail", Description: "根据客户ID查询客户详情", InputSchema: map[string]interface{}{"id": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/customers/{id}", HttpMethod: "GET"},
		{GatewayID: crmGW, ToolName: "query_inventory", Description: "查询库存信息，可按仓库筛选", InputSchema: map[string]interface{}{"warehouse": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/inventory", HttpMethod: "GET"},
		{GatewayID: crmGW, ToolName: "get_inventory_item", Description: "根据SKU查询商品库存详情", InputSchema: map[string]interface{}{"sku": map[string]interface{}{"type": "string"}}, BackendUrl: "http://localhost:9090/api/inventory/{sku}", HttpMethod: "GET"},
	}

	for i := range tools {
		if err := repo.Create(&tools[i]); err != nil {
			log.Printf("创建工具 '%s' 失败（可能已存在）: %v", tools[i].ToolName, err)
		} else {
			log.Printf("✓ [GW:%d] %-22s %s %s", tools[i].GatewayID, tools[i].ToolName, tools[i].HttpMethod, tools[i].BackendUrl)
		}
	}

	// ── 为订单服务网关创建 API Key ──
	orderKey := "mcp-gw-sk-orders-2026"
	if _, err := repo.GetApiKeyByValue(orderKey); err != nil {
		repo.CreateApiKey(&model.ApiKey{GatewayID: orderGW, Key: orderKey, Name: "订单服务演示密钥"})
		log.Printf("✓ 创建 API Key: %s → 订单服务网关", orderKey)
	}

	log.Println("\n种子数据写入完成！")
	log.Println("网关: Default Gateway(公开) / 订单服务(需Key) / 客户与库存(公开)")
}
