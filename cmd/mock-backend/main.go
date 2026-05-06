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

	// ---- 首页 ----
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"service": "模拟企业后端服务 (Mock Backend)",
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
