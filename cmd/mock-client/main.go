// AI 客户端模拟器 —— 测试多网关隔离、工具权限、跨网关调用拒绝
//
// 使用方式:
//
//	go run cmd/mock-client/main.go
//
// 前提：go run cmd/server/main.go  +  go run cmd/mock-backend/main.go  +  go run cmd/seed/main.go
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const gatewayURL = "http://localhost:8080"

var (
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cCyan   = "\033[36m"
	cRed    = "\033[31m"
	cReset  = "\033[0m"
)

type testScenario struct {
	name    string
	sseURL  string // 带 gateway 参数或 api_key 参数
	tools   int    // 期望的工具数量
	allowed []string
	rejected []string
}

func main() {
	log.SetFlags(0)
	printBanner()

	scenarios := []testScenario{
		{
			name:     "Default Gateway (无参数)",
			sseURL:   "/mcp/sse",
			tools:    7,
			allowed:  []string{"query_orders", "query_customers", "query_inventory"},
			rejected: []string{},
		},
		{
			name:     "Default Gateway (gateway=Default Gateway)",
			sseURL:   "/mcp/sse?gateway=Default+Gateway",
			tools:    7,
			allowed:  []string{"query_orders", "get_customer_detail"},
			rejected: []string{},
		},
		{
			name:     "订单服务网关 (gateway=订单服务)",
			sseURL:   "/mcp/sse?gateway=%E8%AE%A2%E5%8D%95%E6%9C%8D%E5%8A%A1",
			tools:    3,
			allowed:  []string{"query_orders", "get_order_detail", "create_order"},
			rejected: []string{"query_customers", "query_inventory"},
		},
		{
			name:     "客户与库存网关 (gateway=客户与库存)",
			sseURL:   "/mcp/sse?gateway=%E5%AE%A2%E6%88%B7%E4%B8%8E%E5%BA%93%E5%AD%98",
			tools:    4,
			allowed:  []string{"query_customers", "get_customer_detail", "query_inventory"},
			rejected: []string{"query_orders", "create_order"},
		},
	}

	allPassed := true
	for _, sc := range scenarios {
		if !runScenario(sc) {
			allPassed = false
		}
	}

	fmt.Println(strings.Repeat("=", 55))
	if allPassed {
		log.Printf("%s  全部场景通过 ✓%s", cGreen, cReset)
	} else {
		log.Printf("%s  存在失败场景 ✗%s", cRed, cReset)
	}
	fmt.Println(strings.Repeat("=", 55))
}

func runScenario(sc testScenario) bool {
	log.Printf("\n%s━━━ %s ━━━%s", cYellow, sc.name, cReset)

	resp, err := http.Get(gatewayURL + sc.sseURL)
	if err != nil {
		log.Printf("%s  ✗ SSE 连接失败: %v%s", cRed, err, cReset)
		return false
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	sessionID := readSessionID(reader)
	responseCh := make(chan string, 16)
	go readSSE(reader, responseCh)
	time.Sleep(200 * time.Millisecond)

	// 握手
	sendReq(sessionID, "initialize", nil, "init")
	waitResp(responseCh, "initialize")

	// 获取工具列表并验证数量
	sendReq(sessionID, "tools/list", nil, "list")
	n := countTools(responseCh)
	if n == sc.tools {
		log.Printf("  %s✓ 工具数量: %d (期望 %d)%s", cGreen, n, sc.tools, cReset)
	} else {
		log.Printf("  %s✗ 工具数量: %d (期望 %d)%s", cRed, n, sc.tools, cReset)
		return false
	}

	// 验证允许的工具能调用
	for _, tool := range sc.allowed {
		if !toolCall(sessionID, tool, true, responseCh) {
			return false
		}
	}

	// 验证跨网关工具被拒绝
	for _, tool := range sc.rejected {
		if !toolCall(sessionID, tool, false, responseCh) {
			return false
		}
	}

	return true
}

func toolCall(sessionID, toolName string, expectSuccess bool, ch <-chan string) bool {
	args := map[string]interface{}{"id": "TEST-001", "customer": "CUST-101", "amount": 100}
	// 根据工具名精简参数
	switch toolName {
	case "query_customers":
		args = map[string]interface{}{"level": "vip"}
	case "query_inventory":
		args = map[string]interface{}{"warehouse": "北京仓"}
	case "query_orders":
		args = map[string]interface{}{"customer": "CUST-101"}
	case "create_order":
		args = map[string]interface{}{"customer": "CUST-101", "amount": 100.0}
	}

	argsJSON, _ := json.Marshal(args)
	params := map[string]interface{}{"name": toolName, "arguments": json.RawMessage(argsJSON)}
	paramsJSON, _ := json.Marshal(params)
	sendReq(sessionID, "tools/call", json.RawMessage(paramsJSON), "call-"+toolName)

	data := waitResp(ch, toolName)
	if data == "" {
		return !expectSuccess
	}

	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal([]byte(data), &rpc)

	if expectSuccess {
		if rpc.Error != nil {
			log.Printf("  %s✗ %s 应成功但返回了错误: %s%s", cRed, toolName, rpc.Error.Message, cReset)
			return false
		}
		log.Printf("  %s✓ %s 调用成功%s", cGreen, toolName, cReset)
	} else {
		if rpc.Error != nil {
			log.Printf("  %s✓ %s 正确被拒绝: %s%s", cGreen, toolName, rpc.Error.Message, cReset)
		} else {
			log.Printf("  %s✗ %s 应被拒绝但调用成功了（跨网关隔离失效）%s", cRed, toolName, cReset)
			return false
		}
	}
	return true
}

func countTools(ch <-chan string) int {
	select {
	case data := <-ch:
		var wrapper struct {
			Result struct {
				Tools []struct{ Name string } `json:"tools"`
			} `json:"result"`
		}
		json.Unmarshal([]byte(data), &wrapper)
		return len(wrapper.Result.Tools)
	case <-time.After(5 * time.Second):
		return 0
	}
}

func printBanner() {
	fmt.Println(strings.Repeat("=", 55))
	fmt.Println("  AI 客户端模拟器 — 多网关隔离测试")
	fmt.Println(strings.Repeat("=", 55))
}

func readSessionID(reader *bufio.Reader) string {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("读取 session_id 失败: %v", err)
		}
		if strings.HasPrefix(line, "data: ") {
			var event map[string]string
			data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			json.Unmarshal([]byte(data), &event)
			if id, ok := event["session_id"]; ok {
				return id
			}
		}
	}
}

func readSSE(reader *bufio.Reader, ch chan<- string) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[SSE] 读取错误: %v", err)
			}
			return
		}
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
			var temp map[string]string
			if json.Unmarshal([]byte(data), &temp) == nil {
				if _, ok := temp["session_id"]; ok {
					continue
				}
			}
			ch <- data
		}
	}
}

func sendReq(sessionID, method string, params json.RawMessage, id string) {
	req := map[string]interface{}{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	body, _ := json.Marshal(req)
	resp, _ := http.Post(
		fmt.Sprintf("%s/mcp/message?session_id=%s", gatewayURL, sessionID),
		"application/json", bytes.NewReader(body),
	)
	if resp != nil {
		resp.Body.Close()
	}
}

func waitResp(ch <-chan string, context string) string {
	select {
	case data := <-ch:
		return data
	case <-time.After(5 * time.Second):
		log.Printf("  %s✗ 超时%s", cYellow, cReset)
		return ""
	}
}
