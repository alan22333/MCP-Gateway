// 种子数据工具 —— 向数据库中写入模拟的企业工具配置
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

	// 定义模拟工具配置 —— 对应 mock-backend 暴露的各个 API
	tools := []model.ApiTool{
		{
			ToolName:    "query_orders",
			Description: "查询订单列表。可按客户ID(customer)和订单状态(status)筛选。状态包括: 待支付、已发货、已完成、退款中",
			InputSchema: map[string]interface{}{
				"customer": map[string]interface{}{"type": "string", "description": "客户ID，如 CUST-101"},
				"status":   map[string]interface{}{"type": "string", "description": "订单状态"},
			},
			BackendUrl: "http://localhost:9090/api/orders",
			Enabled: true,
			HttpMethod: "GET",
		},
		{
			ToolName:    "get_order_detail",
			Description: "根据订单ID查询单个订单的详细信息，包括金额、客户、状态和创建时间",
			InputSchema: map[string]interface{}{
				"id": map[string]interface{}{"type": "string", "description": "订单ID，如 ORD-001"},
			},
			BackendUrl: "http://localhost:9090/api/orders/{id}",
			Enabled: true,
			HttpMethod: "GET",
		},
		{
			ToolName:    "create_order",
			Description: "创建一个新订单。需要提供客户ID和订单金额",
			InputSchema: map[string]interface{}{
				"customer": map[string]interface{}{"type": "string", "description": "客户ID"},
				"amount":   map[string]interface{}{"type": "number", "description": "订单金额"},
			},
			BackendUrl: "http://localhost:9090/api/orders",
			Enabled: true,
			HttpMethod: "POST",
		},
		{
			ToolName:    "query_customers",
			Description: "查询客户列表。可按客户等级(level)筛选: vip / normal",
			InputSchema: map[string]interface{}{
				"level": map[string]interface{}{"type": "string", "description": "客户等级: vip 或 normal"},
			},
			BackendUrl: "http://localhost:9090/api/customers",
			Enabled: true,
			HttpMethod: "GET",
		},
		{
			ToolName:    "get_customer_detail",
			Description: "根据客户ID查询单个客户的详细信息，包括姓名、等级和联系方式",
			InputSchema: map[string]interface{}{
				"id": map[string]interface{}{"type": "string", "description": "客户ID，如 CUST-101"},
			},
			BackendUrl: "http://localhost:9090/api/customers/{id}",
			Enabled: true,
			HttpMethod: "GET",
		},
		{
			ToolName:    "query_inventory",
			Description: "查询库存信息。可按仓库名称(warehouse)筛选: 北京仓、上海仓、深圳仓",
			InputSchema: map[string]interface{}{
				"warehouse": map[string]interface{}{"type": "string", "description": "仓库名称"},
			},
			BackendUrl: "http://localhost:9090/api/inventory",
			Enabled: true,
			HttpMethod: "GET",
		},
		{
			ToolName:    "get_inventory_item",
			Description: "根据SKU编码查询单个商品的库存详情，包括库存数量和所在仓库",
			InputSchema: map[string]interface{}{
				"sku": map[string]interface{}{"type": "string", "description": "商品SKU，如 SKU-1001"},
			},
			BackendUrl: "http://localhost:9090/api/inventory/{sku}",
			Enabled: true,
			HttpMethod: "GET",
		},
	}

	for i := range tools {
		if err := repo.Create(&tools[i]); err != nil {
			log.Printf("创建工具 '%s' 失败（可能已存在）: %v", tools[i].ToolName, err)
		} else {
			log.Printf("✓ 已注册工具: %-25s %s %s", tools[i].ToolName, tools[i].HttpMethod, tools[i].BackendUrl)
		}
	}

	log.Println("\n种子数据写入完成！")
}
