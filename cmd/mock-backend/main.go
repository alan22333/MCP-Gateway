// 模拟企业后端服务 —— 提供订单、用户、库存等常见业务 API
// 用于在本地测试 MCP Gateway 的协议转换与代理能力
package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/gin-gonic/gin"
)

// openapiSpec 返回此 mock 后端的 OpenAPI 3.0 文档（用于测试导入功能）
func openapiSpec() gin.H {
	return gin.H{
		"openapi": "3.0.0",
		"info":   gin.H{"title": "企业业务系统 API", "version": "1.0.0", "description": "模拟后端——订单、客户、库存管理"},
		"servers": []gin.H{{"url": "http://localhost:9090", "description": "本地开发服务器"}},
		"paths": gin.H{
			"/api/orders": gin.H{
				"get": gin.H{
					"operationId": "query_orders",
					"summary":     "查询订单列表",
					"description": "按客户ID和订单状态筛选订单。状态: 待支付、已发货、已完成、退款中",
					"parameters": []gin.H{
						{"name": "customer", "in": "query", "schema": gin.H{"type": "string"}, "description": "客户ID，如 CUST-101"},
						{"name": "status", "in": "query", "schema": gin.H{"type": "string"}, "description": "订单状态"},
					},
				},
				"post": gin.H{
					"operationId": "create_order",
					"summary":     "创建新订单",
					"description": "创建订单，需提供客户ID和金额",
					"requestBody": gin.H{
						"content": gin.H{"application/json": gin.H{"schema": gin.H{
							"type": "object",
							"properties": gin.H{
								"customer": gin.H{"type": "string", "description": "客户ID"},
								"amount":   gin.H{"type": "number", "description": "订单金额"},
							},
							"required": []string{"customer", "amount"},
						}}},
					},
				},
			},
			"/api/orders/{id}": gin.H{
				"get": gin.H{
					"operationId": "get_order_detail",
					"summary":     "查询订单详情",
					"description": "根据订单ID查询单个订单",
					"parameters":  []gin.H{{"name": "id", "in": "path", "required": true, "schema": gin.H{"type": "string"}, "description": "订单ID，如 ORD-001"}},
				},
			},
			"/api/customers": gin.H{
				"get": gin.H{
					"operationId": "query_customers",
					"summary":     "查询客户列表",
					"description": "按客户等级筛选。等级: vip, normal",
					"parameters":  []gin.H{{"name": "level", "in": "query", "schema": gin.H{"type": "string"}, "description": "客户等级: vip 或 normal"}},
				},
			},
			"/api/customers/{id}": gin.H{
				"get": gin.H{
					"operationId": "get_customer_detail",
					"summary":     "查询客户详情",
					"description": "根据客户ID查询单个客户信息",
					"parameters":  []gin.H{{"name": "id", "in": "path", "required": true, "schema": gin.H{"type": "string"}, "description": "客户ID，如 CUST-101"}},
				},
			},
			"/api/inventory": gin.H{
				"get": gin.H{
					"operationId": "query_inventory",
					"summary":     "查询库存列表",
					"description": "按仓库名称筛选库存。仓库: 北京仓、上海仓、深圳仓",
					"parameters":  []gin.H{{"name": "warehouse", "in": "query", "schema": gin.H{"type": "string"}, "description": "仓库名称"}},
				},
			},
			"/api/inventory/{sku}": gin.H{
				"get": gin.H{
					"operationId": "get_inventory_item",
					"summary":     "查询库存详情",
					"description": "根据SKU查询单个商品库存",
					"parameters":  []gin.H{{"name": "sku", "in": "path", "required": true, "schema": gin.H{"type": "string"}, "description": "商品SKU，如 SKU-1001"}},
				},
			},
		},
	}
}

// ---------- 模拟数据 ----------

type Order struct {
	OrderID   string  `json:"order_id"`
	Customer  string  `json:"customer"`
	Amount    float64 `json:"amount"`
	Status    string  `json:"status"`
	CreatedAt string  `json:"created_at"`
}

type Customer struct {
	CustomerID string `json:"customer_id"`
	Name       string `json:"name"`
	Level      string `json:"level"`
	Phone      string `json:"phone"`
}

type Inventory struct {
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	Warehouse string `json:"warehouse"`
}

var orders = []Order{
	{OrderID: "ORD-001", Customer: "CUST-101", Amount: 299.00, Status: "已发货", CreatedAt: "2026-04-20"},
	{OrderID: "ORD-002", Customer: "CUST-102", Amount: 1599.50, Status: "待支付", CreatedAt: "2026-04-25"},
	{OrderID: "ORD-003", Customer: "CUST-101", Amount: 89.90, Status: "已完成", CreatedAt: "2026-04-18"},
	{OrderID: "ORD-004", Customer: "CUST-103", Amount: 4200.00, Status: "已发货", CreatedAt: "2026-04-28"},
	{OrderID: "ORD-005", Customer: "CUST-102", Amount: 199.00, Status: "退款中", CreatedAt: "2026-04-29"},
}

var customers = []Customer{
	{CustomerID: "CUST-101", Name: "张三", Level: "vip", Phone: "13800001001"},
	{CustomerID: "CUST-102", Name: "李四", Level: "normal", Phone: "13800001002"},
	{CustomerID: "CUST-103", Name: "王五", Level: "vip", Phone: "13800001003"},
}

var inventory = []Inventory{
	{SKU: "SKU-1001", Name: "机械键盘 K8", Quantity: 128, Warehouse: "北京仓"},
	{SKU: "SKU-1002", Name: "无线鼠标 M3", Quantity: 56, Warehouse: "上海仓"},
	{SKU: "SKU-1003", Name: "27寸 4K 显示器", Quantity: 12, Warehouse: "深圳仓"},
	{SKU: "SKU-1004", Name: "Type-C 拓展坞", Quantity: 203, Warehouse: "北京仓"},
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

// ---------- 入口 ----------

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// CORS 中间件 —— 允许前端从其他端口访问
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// ---- OpenAPI 文档端点（供网关导入功能测试）----
	r.GET("/openapi.json", func(c *gin.Context) {
		c.JSON(200, openapiSpec())
	})
	r.GET("/swagger.json", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"swagger":  "2.0",
			"info":     gin.H{"title": "企业业务系统 API (Swagger 2.0)", "version": "1.0.0"},
			"host":     "localhost:9090",
			"basePath": "/",
			"schemes":  []string{"http"},
			"paths": gin.H{
				"/api/customers": gin.H{
					"get": gin.H{
						"operationId": "query_customers",
						"summary":     "查询客户列表",
						"parameters":  []gin.H{{"name": "level", "in": "query", "type": "string", "description": "客户等级"}},
					},
				},
			},
		})
	})

	// ---- 首页 ----
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": "模拟企业后端服务 (Mock Backend)",
			"openapi_specs": []string{
				"GET /openapi.json    ← OpenAPI 3.0 文档（可用于网关导入测试）",
				"GET /swagger.json    ← Swagger 2.0 文档（可用于网关导入测试）",
			},
			"endpoints": []string{
				"GET  /api/orders?customer=&status=",
				"GET  /api/orders/:id",
				"POST /api/orders",
				"GET  /api/customers?level=vip",
				"GET  /api/customers/:id",
				"GET  /api/inventory?warehouse=",
				"GET  /api/inventory/:sku",
			},
		})
	})

	// ---- 订单 API ----
	r.GET("/api/orders", func(c *gin.Context) {
		customer := c.Query("customer")
		status := c.Query("status")
		var result []Order
		for _, o := range orders {
			if customer != "" && o.Customer != customer {
				continue
			}
			if status != "" && o.Status != status {
				continue
			}
			result = append(result, o)
		}
		c.JSON(200, gin.H{"total": len(result), "orders": result})
	})

	r.GET("/api/orders/:id", func(c *gin.Context) {
		id := c.Param("id")
		for _, o := range orders {
			if o.OrderID == id {
				c.JSON(200, o)
				return
			}
		}
		c.JSON(404, gin.H{"error": "订单不存在"})
	})

	r.POST("/api/orders", func(c *gin.Context) {
		var input struct {
			Customer string  `json:"customer" binding:"required"`
			Amount   float64 `json:"amount" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		newOrder := Order{
			OrderID:   fmt.Sprintf("ORD-%03d", rand.Intn(900)+100),
			Customer:  input.Customer,
			Amount:    input.Amount,
			Status:    "待支付",
			CreatedAt: time.Now().Format("2006-01-02"),
		}
		orders = append(orders, newOrder)
		c.JSON(201, newOrder)
	})

	// ---- 客户 API ----
	r.GET("/api/customers", func(c *gin.Context) {
		level := c.Query("level")
		var result []Customer
		for _, cst := range customers {
			if level != "" && cst.Level != level {
				continue
			}
			result = append(result, cst)
		}
		c.JSON(200, gin.H{"total": len(result), "customers": result})
	})

	r.GET("/api/customers/:id", func(c *gin.Context) {
		id := c.Param("id")
		for _, cst := range customers {
			if cst.CustomerID == id {
				c.JSON(200, cst)
				return
			}
		}
		c.JSON(404, gin.H{"error": "客户不存在"})
	})

	// ---- 库存 API ----
	r.GET("/api/inventory", func(c *gin.Context) {
		warehouse := c.Query("warehouse")
		var result []Inventory
		for _, inv := range inventory {
			if warehouse != "" && inv.Warehouse != warehouse {
				continue
			}
			result = append(result, inv)
		}
		c.JSON(200, gin.H{"total": len(result), "items": result})
	})

	r.GET("/api/inventory/:sku", func(c *gin.Context) {
		sku := c.Param("sku")
		for _, inv := range inventory {
			if inv.SKU == sku {
				c.JSON(200, inv)
				return
			}
		}
		c.JSON(404, gin.H{"error": "SKU 不存在"})
	})

	log.Println("=== 模拟企业后端 ===")
	for _, item := range r.Routes() {
		log.Printf("  %s %s", item.Method, item.Path)
	}
	log.Println("监听 :9090")
	log.Fatal(r.Run(":9090"))
}
