// AI 客户端模拟器 —— 模拟大模型与 MCP Gateway 的完整交互流程
//
// 使用方式:
//
//	go run cmd/mock-client/main.go
//
// 前提：需要先启动 gateway (go run cmd/server/main.go)
// 和 mock-backend (go run cmd/mock-backend/main.go)
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
	cReset  = "\033[0m"
)

func main() {
	log.SetFlags(0)
	printBanner()

	// Step 1: 建立 SSE 连接，获取 session_id
	resp, err := http.Get(gatewayURL + "/mcp/sse")
	if err != nil {
		log.Fatalf("SSE 连接失败: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	sessionID := readSessionID(reader)
	log.Printf("%s✓ SSE 会话建立: %s%s\n", cGreen, sessionID, cReset)

	// Step 2: 启动后台 goroutine 读取 SSE 响应
	responseCh := make(chan string, 16)
	go readSSE(reader, responseCh)

	// 等待 SSE 流稳定
	time.Sleep(200 * time.Millisecond)

	// Step 3: 握手 → 获取工具列表 → 调用工具
	sendReq(sessionID, "initialize", nil, "1")
	waitResp(responseCh, "initialize")

	sendReq(sessionID, "tools/list", nil, "2")
	waitResp(responseCh, "tools/list")

	// 模拟 AI 调用多种工具
	callTool(sessionID, "query_customers", map[string]interface{}{"level": "vip"}, responseCh)
	callTool(sessionID, "get_order_detail", map[string]interface{}{"id": "ORD-001"}, responseCh)
	callTool(sessionID, "query_inventory", map[string]interface{}{"warehouse": "北京仓"}, responseCh)
	callTool(sessionID, "create_order", map[string]interface{}{"customer": "CUST-101", "amount": 599.00}, responseCh)
	callTool(sessionID, "query_orders", map[string]interface{}{"customer": "CUST-102"}, responseCh)

	log.Printf("\n%s══════════════════════════════════════%s", cGreen, cReset)
	log.Printf("%s  端到端测试全部完成 ✓%s", cGreen, cReset)
	log.Printf("%s══════════════════════════════════════%s\n", cGreen, cReset)
}

func printBanner() {
	fmt.Println(strings.Repeat("=", 55))
	fmt.Println("  AI 客户端模拟器 — MCP Gateway E2E 测试")
	fmt.Println(strings.Repeat("=", 55))
}

// readSessionID 从 SSE 事件流中读取第一条事件，提取 session_id
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

// readSSE 后台持续读取 SSE 事件，推送到 channel
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
			// 跳过 session_id 事件（已处理）
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

// sendReq 发送 JSON-RPC 请求到网关
func sendReq(sessionID, method string, params json.RawMessage, id string) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(
		fmt.Sprintf("%s/mcp/message?session_id=%s", gatewayURL, sessionID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		log.Printf("  ✗ 发送失败: %v", err)
		return
	}
	resp.Body.Close()
}

// callTool 调用一个工具并等待响应
func callTool(sessionID, toolName string, args map[string]interface{}, ch <-chan string) {
	log.Printf("\n%s── 调用工具: %s ──%s", cYellow, toolName, cReset)

	argsJSON, _ := json.Marshal(args)
	params := map[string]interface{}{
		"name":      toolName,
		"arguments": json.RawMessage(argsJSON),
	}
	paramsJSON, _ := json.Marshal(params)

	sendReq(sessionID, "tools/call", json.RawMessage(paramsJSON), "call-"+toolName)
	waitResp(ch, toolName)
}

// waitResp 等待一个 SSE 响应并格式化输出
func waitResp(ch <-chan string, context string) {
	select {
	case data := <-ch:
		printResult(data, context)
	case <-time.After(5 * time.Second):
		log.Printf("  %s✗ 超时：5s 内未收到响应%s", cYellow, cReset)
	}
}

// printResult 格式化打印 JSON-RPC 响应
func printResult(data string, context string) {
	var rpc struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &rpc); err != nil {
		log.Printf("  解析失败: %v", err)
		return
	}

	if rpc.Error != nil {
		log.Printf("  %s✗ 错误 [%d]: %s%s", cYellow, rpc.Error.Code, rpc.Error.Message, cReset)
		return
	}

	switch context {
	case "initialize":
		var result map[string]interface{}
		json.Unmarshal(rpc.Result, &result)
		pretty, _ := json.MarshalIndent(result, "  ", "  ")
		log.Printf("  %s✓ 握手成功%s\n  %s", cGreen, cReset, string(pretty))

	case "tools/list":
		var result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		}
		json.Unmarshal(rpc.Result, &result)
		log.Printf("  %s✓ 获取到 %d 个可用工具:%s", cGreen, len(result.Tools), cReset)
		for _, t := range result.Tools {
			log.Printf("    • %s%-25s%s %s", cCyan, t.Name, cReset, t.Description)
		}

	default:
		// tools/call 返回的是 CallToolResult
		var callResult struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(rpc.Result, &callResult); err != nil {
			pretty, _ := json.MarshalIndent(rpc.Result, "  ", "  ")
			log.Printf("  %s✓ 响应:%s\n  %s", cGreen, cReset, string(pretty))
			return
		}
		if len(callResult.Content) > 0 && callResult.Content[0].Type == "text" {
			// 尝试格式化 text 中的 JSON
			var formatted interface{}
			if json.Unmarshal([]byte(callResult.Content[0].Text), &formatted) == nil {
				pretty, _ := json.MarshalIndent(formatted, "  ", "  ")
				log.Printf("  %s✓ 调用成功 →%s\n  %s", cGreen, cReset, string(pretty))
			} else {
				log.Printf("  %s✓ 调用成功 →%s %s", cGreen, cReset, callResult.Content[0].Text)
			}
		}
	}
}
