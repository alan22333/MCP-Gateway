// AI 客户端模拟器 —— 测试 SSE 旧传输 + Streamable HTTP 新传输 + 多网关隔离
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
	name     string
	sseURL   string // 带 gateway 参数或 api_key 参数（旧 SSE 传输用）
	tools    int    // 期望的工具数量
	allowed  []string
	rejected []string
}

func main() {
	log.SetFlags(0)
	printBanner()

	// ── Part 1: 旧版 SSE 传输 ──
	log.Printf("%s━━━ Part 1: 旧版 SSE 传输 (GET /mcp/sse + POST /mcp/message) ━━━%s", cCyan, cReset)
	scenarios := []testScenario{
		{
			name:     "Default Gateway (无参数)",
			sseURL:   "/mcp/sse",
			tools:    5,
			allowed:  []string{"query_orders", "query_customers", "query_inventory"},
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
			tools:    3,
			allowed:  []string{"query_customers", "get_inventory_item", "query_inventory"},
			rejected: []string{"query_orders", "create_order"},
		},
	}

	allPassed := true
	for _, sc := range scenarios {
		if !runSSEScenario(sc) {
			allPassed = false
		}
	}

	// ── Part 2: 新版 Streamable HTTP 传输 ──
	log.Printf("\n%s━━━ Part 2: Streamable HTTP 传输 (POST /mcp) ━━━%s", cCyan, cReset)
	if !runStreamableStateless() {
		allPassed = false
	}
	if !runStreamableSession() {
		allPassed = false
	}
	if !runStreamableSSE() {
		allPassed = false
	}
	if !runStreamableGatewayIsolation() {
		allPassed = false
	}

	fmt.Println(strings.Repeat("=", 55))
	if allPassed {
		log.Printf("%s  全部场景通过 ✓%s", cGreen, cReset)
	} else {
		log.Printf("%s  存在失败场景 ✗%s", cRed, cReset)
	}
	fmt.Println(strings.Repeat("=", 55))
}

// ====== 旧版 SSE 传输 ======

func runSSEScenario(sc testScenario) bool {
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
	sendReqSSE(sessionID, "initialize", nil, "init")
	waitResp(responseCh, "initialize")

	// 工具列表验证
	sendReqSSE(sessionID, "tools/list", nil, "list")
	n := countTools(responseCh)
	if n == sc.tools {
		log.Printf("  %s✓ 工具数量: %d (期望 %d)%s", cGreen, n, sc.tools, cReset)
	} else {
		log.Printf("  %s✗ 工具数量: %d (期望 %d)%s", cRed, n, sc.tools, cReset)
		return false
	}

	// 允许的工具可调用
	for _, tool := range sc.allowed {
		if !toolCallSSE(sessionID, tool, true, responseCh) {
			return false
		}
	}

	// 跨网关工具被拒绝
	for _, tool := range sc.rejected {
		if !toolCallSSE(sessionID, tool, false, responseCh) {
			return false
		}
	}

	return true
}

// ====== Streamable HTTP: 无状态模式 ======

func runStreamableStateless() bool {
	log.Printf("\n%s━━━ Streamable: 无状态 JSON 模式 ━━━%s", cYellow, cReset)

	// initialize
	initResp := postMCP("", "initialize", "1", "", nil)
	if initResp == nil {
		log.Printf("  %s✗ initialize 失败%s", cRed, cReset)
		return false
	}
	if initResp.Error != nil {
		log.Printf("  %s✗ initialize 返回错误: %s%s", cRed, initResp.Error.Message, cReset)
		return false
	}
	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	json.Unmarshal(initResp.Result, &initResult)
	if initResult.ProtocolVersion != "2025-03-26" {
		log.Printf("  %s✗ protocolVersion 不匹配: %s%s", cRed, initResult.ProtocolVersion, cReset)
		return false
	}
	log.Printf("  %s✓ initialize: protocol=%s%s", cGreen, initResult.ProtocolVersion, cReset)

	// tools/list（无 session，新请求创建新 session）
	listResp := postMCP("", "tools/list", "2", "", nil)
	if listResp == nil || listResp.Error != nil {
		log.Printf("  %s✗ tools/list 失败%s", cRed, cReset)
		return false
	}
	var listResult struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	json.Unmarshal(listResp.Result, &listResult)
	if len(listResult.Tools) != 5 {
		log.Printf("  %s✗ 工具数量: %d (期望 5)%s", cRed, len(listResult.Tools), cReset)
		return false
	}
	log.Printf("  %s✓ tools/list: %d 个工具%s", cGreen, len(listResult.Tools), cReset)

	return true
}

// ====== Streamable HTTP: 会话模式 ======

func runStreamableSession() bool {
	log.Printf("\n%s━━━ Streamable: 会话模式 (Mcp-Session-Id) ━━━%s", cYellow, cReset)

	// Step 1: initialize → 获取 session ID
	resp, sessionID := postMCPWithSession("", "initialize", "1", "", nil)
	if resp == nil || resp.Error != nil {
		log.Printf("  %s✗ initialize 失败%s", cRed, cReset)
		return false
	}
	if sessionID == "" {
		log.Printf("  %s✗ 未收到 Mcp-Session-Id%s", cRed, cReset)
		return false
	}
	log.Printf("  %s✓ session 创建: %s%s", cGreen, sessionID, cReset)

	// Step 2: tools/list → 复用 session
	resp2, _ := postMCPWithSession(sessionID, "tools/list", "2", "", nil)
	if resp2 == nil || resp2.Error != nil {
		log.Printf("  %s✗ tools/list (session) 失败%s", cRed, cReset)
		return false
	}

	// Step 3: notification → 202
	resp3, _ := postMCPWithSession(sessionID, "notifications/initialized", "", "", nil)
	if resp3 == nil {
		// 202 没有 body，resp3 为 nil 是正常的（notification 不返回 JSON-RPC 响应）
		log.Printf("  %s✓ notifications/initialized (202)%s", cGreen, cReset)
	}

	log.Printf("  %s✓ 会话复用成功%s", cGreen, cReset)
	return true
}

// ====== Streamable HTTP: SSE 流式响应 ======

func runStreamableSSE() bool {
	log.Printf("\n%s━━━ Streamable: SSE 流式响应 (Accept: text/event-stream) ━━━%s", cYellow, cReset)

	body := `{"jsonrpc":"2.0","id":"1","method":"tools/list"}`
	req, _ := http.NewRequest("POST", gatewayURL+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("  %s✗ SSE 请求失败: %v%s", cRed, err, cReset)
		return false
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		log.Printf("  %s✗ Content-Type 不是 text/event-stream: %s%s", cRed, contentType, cReset)
		return false
	}

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("  %s✗ 读取 SSE 失败: %v%s", cRed, err, cReset)
		return false
	}

	if strings.HasPrefix(line, "data: ") {
		var rpcResp struct {
			Result json.RawMessage `json:"result"`
		}
		json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &rpcResp)
		var listResult struct {
			Tools []struct{ Name string } `json:"tools"`
		}
		json.Unmarshal(rpcResp.Result, &listResult)
		log.Printf("  %s✓ SSE 流式响应: %d 个工具%s", cGreen, len(listResult.Tools), cReset)
	} else {
		log.Printf("  %s✗ 不是 SSE data 行: %s%s", cRed, line, cReset)
		return false
	}

	return true
}

// ====== Streamable HTTP: 多网关隔离 ======

func runStreamableGatewayIsolation() bool {
	log.Printf("\n%s━━━ Streamable: 多网关隔离 ━━━%s", cYellow, cReset)

	// 订单服务网关
	resp, _ := postMCPWithSession("", "initialize", "1", "订单服务", nil)
	if resp == nil || resp.Error != nil {
		log.Printf("  %s✗ 连接订单服务网关失败%s", cRed, cReset)
		return false
	}

	// tools/list 应只返回 3 个工具
	listResp := postMCP("", "tools/list", "2", "订单服务", nil)
	if listResp == nil || listResp.Error != nil {
		log.Printf("  %s✗ tools/list 失败%s", cRed, cReset)
		return false
	}
	var listResult struct {
		Tools []struct{ Name string } `json:"tools"`
	}
	json.Unmarshal(listResp.Result, &listResult)
	if len(listResult.Tools) != 3 {
		log.Printf("  %s✗ 订单服务工具数量: %d (期望 3)%s", cRed, len(listResult.Tools), cReset)
		return false
	}

	// 跨网关调用应被拒绝
	callResp := postMCP("", "tools/call", "3", "订单服务", map[string]interface{}{
		"name":      "query_orders",
		"arguments": map[string]string{"customer": "CUST-101"},
	})
	if callResp == nil || callResp.Error == nil {
		log.Printf("  %s✗ 跨网关调用未被拒绝%s", cRed, cReset)
		return false
	}
	log.Printf("  %s✓ 订单服务网关: 3 个工具, 跨网关调用被拒绝%s", cGreen, cReset)

	return true
}

// ====== Streamable HTTP 辅助函数 ======

// postMCP 向 POST /mcp 发送请求并解析 JSON-RPC 响应
// postMCP 发送 Streamable HTTP 请求，gatewayName 为空表示使用默认网关
func postMCP(sessionID, method, id string, gatewayName string, extraFields map[string]interface{}) *rpcResponse {
	resp, _ := postMCPWithSession(sessionID, method, id, gatewayName, extraFields)
	return resp
}

func postMCPWithSession(sessionID, method, id string, gatewayName string, extraFields map[string]interface{}) (*rpcResponse, string) {
	reqMap := map[string]interface{}{"jsonrpc": "2.0", "method": method}
	if id != "" {
		reqMap["id"] = id
	}
	if extraFields != nil {
		for k, v := range extraFields {
			reqMap[k] = v
		}
	}
	body, _ := json.Marshal(reqMap)

	url := gatewayURL + "/mcp"
	if gatewayName != "" {
		url += "?gateway=" + gatewayName
	}

	httpReq, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", sessionID)
	}

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Printf("  %s✗ HTTP 请求失败: %v%s", cRed, err, cReset)
		return nil, ""
	}
	defer httpResp.Body.Close()

	// 读取响应头中的 session ID
	newSessionID := httpResp.Header.Get("Mcp-Session-Id")

	// Notification 返回 202（无 body）
	if httpResp.StatusCode == 202 {
		return &rpcResponse{}, newSessionID
	}

	respBody, _ := io.ReadAll(httpResp.Body)
	var rpcResp rpcResponse
	if json.Unmarshal(respBody, &rpcResp) != nil {
		return nil, newSessionID
	}
	return &rpcResp, newSessionID
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ====== 旧 SSE 传输辅助函数 ======

func toolCallSSE(sessionID, toolName string, expectSuccess bool, ch <-chan string) bool {
	args := map[string]interface{}{"id": "TEST-001", "customer": "CUST-101", "amount": 100}
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
	sendReqSSE(sessionID, "tools/call", json.RawMessage(paramsJSON), "call-"+toolName)

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
	fmt.Println("  AI 客户端模拟器 — SSE + Streamable HTTP 双传输测试")
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

func sendReqSSE(sessionID, method string, params json.RawMessage, id string) {
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
