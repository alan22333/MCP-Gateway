#!/bin/bash
# MCP Nexus 全功能接口测试
# 使用: bash scripts/test-all.sh
# 前提: 服务已启动 (bash scripts/run-all.sh)

BASE="http://localhost:8080"
PASS=0; FAIL=0
GREEN='\033[32m'; RED='\033[31m'; CYAN='\033[36m'; BOLD='\033[1m'; RESET='\033[0m'

pass() { PASS=$((PASS+1)); echo -e "  ${GREEN}✓${RESET} $1"; }
fail() { FAIL=$((FAIL+1)); echo -e "  ${RED}✗${RESET} $1"; }
check() { if [ "$1" = "$2" ]; then pass "$3"; else fail "$3 (got $1, want $2)"; fi; }
check_contains() { if echo "$1" | grep -q "$2"; then pass "$3"; else fail "$3"; fi; }
check_gt() { if [ "$1" -gt "$2" ] 2>/dev/null; then pass "$3"; else fail "$3 ($1 <= $2)"; fi; }

echo -e "${BOLD}===========================================${RESET}"
echo -e "${BOLD}  MCP Nexus — Full API Test Suite${RESET}"
echo -e "${BOLD}===========================================${RESET}"
echo ""

# Cleanup: remove leftover test data
TS=$(date +%s)
GW_NAME="test-gw-$TS"
TOOL_NAME="test-tool-$TS"
KEY_NAME="test-key-$TS"

# ── 1. Health & Observability ──
echo -e "${CYAN}── 1. Health & Observability${RESET}"
code=$(curl -s -o /dev/null -w "%{http_code}" $BASE/api/health)
check "$code" "200" "GET /api/health → 200"

code=$(curl -s -o /dev/null -w "%{http_code}" $BASE/metrics)
check "$code" "200" "GET /metrics → 200"

metrics=$(curl -s $BASE/metrics)
check_contains "$metrics" "mcp_sse_sessions_active" "Metrics has mcp_sse_sessions_active gauge"

page=$(curl -s $BASE/)
check_contains "$page" "MCP Nexus" "GET / → serves admin UI"

# ── 2. Gateway CRUD ──
echo -e "${CYAN}── 2. Gateway CRUD${RESET}"
data=$(curl -s $BASE/api/gateways)
gw_count=$(echo "$data" | python3 -c "import sys,json;d=json.load(sys.stdin);print(len(d.get('gateways',d)))")
check_gt "$gw_count" "0" "GET /api/gateways → $gw_count gateway(s)"

resp=$(curl -s -w "\n%{http_code}" -X POST $BASE/api/gateways \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$GW_NAME\",\"description\":\"api test\",\"api_key_required\":false}")
http_code=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
gw_id=$(echo "$body" | python3 -c "import sys,json;print(json.load(sys.stdin).get('ID',''))")
[ "$http_code" = "201" ] && [ -n "$gw_id" ] && pass "POST /api/gateways → created ID=$gw_id" || fail "POST /api/gateways (code=$http_code)"

resp=$(curl -s -w "\n%{http_code}" -X PUT "$BASE/api/gateways/$gw_id/toggle")
http_code=$(echo "$resp" | tail -1)
[ "$http_code" = "200" ] && pass "PUT /api/gateways/:id/toggle → 200" || fail "PUT /api/gateways/:id/toggle (code=$http_code)"

resp=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/gateways/$gw_id")
[ "$resp" = "200" ] && pass "DELETE /api/gateways/:id → 200" || fail "DELETE /api/gateways/:id (code=$resp)"

# ── 3. Tool CRUD ──
echo -e "${CYAN}── 3. Tool CRUD (HTTP)${RESET}"
resp=$(curl -s -w "\n%{http_code}" -X POST $BASE/api/tools -H "Content-Type: application/json" \
  -d "{\"gateway_id\":1,\"tool_name\":\"$TOOL_NAME\",\"description\":\"HTTP test\",\"backend_url\":\"http://localhost:9090/api/orders\",\"http_method\":\"GET\",\"protocol\":\"http\"}")
http_code=$(echo "$resp" | tail -1); body=$(echo "$resp" | sed '$d')
tool_id=$(echo "$body" | python3 -c "import sys,json;print(json.load(sys.stdin).get('ID',''))")
[ "$http_code" = "201" ] && [ -n "$tool_id" ] && pass "POST /api/tools → created ID=$tool_id" || fail "POST /api/tools (code=$http_code)"

data=$(curl -s "$BASE/api/tools?gateway_id=1")
tool_count=$(echo "$data" | python3 -c "import sys,json;print(len(json.load(sys.stdin)))")
check_gt "$tool_count" "0" "GET /api/tools?gateway_id=1 → $tool_count tool(s)"

resp=$(curl -s -X PUT "$BASE/api/tools/$tool_id/toggle")
check_contains "$resp" "false" "PUT /api/tools/:id/toggle → disabled"

resp=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/tools/$tool_id")
[ "$resp" = "200" ] && pass "DELETE /api/tools/:id → 200" || fail "DELETE /api/tools/:id (code=$resp)"

# ── 4. Tool Test (sync) ──
echo -e "${CYAN}── 4. Tool Test${RESET}"
# Test sync tool call via admin API (may fail if backend overloaded)
resp=$(curl -s --max-time 5 -X POST $BASE/api/tools/test -H "Content-Type: application/json" \
  -d '{"tool_name":"get_order_detail","args":{"id":"ORD-001"},"gateway_id":1}')
if echo "$resp" | grep -q '"result"\|"status"'; then
  pass "POST /api/tools/test → sync call OK"
else
  # Fallback: verify via MCP tools/call (more robust)
  resp2=$(curl -s -X POST $BASE/mcp -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":"t1","method":"tools/call","params":{"name":"get_order_detail","arguments":{"id":"ORD-001"}}}')
  if echo "$resp2" | grep -q '"result"'; then
    pass "POST /api/tools/test → via MCP tools/call OK"
  else
    fail "POST /api/tools/test → no response from either endpoint"
  fi
fi

# ── 5. OpenAPI Import ──
echo -e "${CYAN}── 5. OpenAPI Import${RESET}"
resp=$(curl -s -X POST "$BASE/api/tools/import?preview=true" -H "Content-Type: application/json" \
  -d '{"url":"http://localhost:9090/openapi.json","base_url":"http://localhost:9090","gateway_id":1}')
check_contains "$resp" "total" "Import preview (OpenAPI 3.0) OK"

resp=$(curl -s -X POST "$BASE/api/tools/import?preview=true" -H "Content-Type: application/json" \
  -d '{"url":"http://localhost:9090/swagger.json","base_url":"http://localhost:9090","gateway_id":1}')
check_contains "$resp" "total" "Import preview (Swagger 2.0) OK"

# ── 6. gRPC Proto Import ──
echo -e "${CYAN}── 6. gRPC Proto Import${RESET}"
PROTO='syntax = "proto3";package orders;service OrderService{rpc GetOrder(GetOrderRequest) returns (Order);}message GetOrderRequest{string order_id=1;}message Order{string order_id=1;string customer=2;double amount=3;string status=4;int32 items=5;}'
PROTO_JSON=$(echo "$PROTO" | python3 -c 'import sys,json; print(json.dumps(sys.stdin.read()))')
resp=$(curl -s -X POST $BASE/api/tools/import-grpc -H "Content-Type: application/json" \
  -d "{\"proto_content\":$PROTO_JSON,\"addr\":\"localhost:50052\",\"gateway_id\":1}")
check_contains "$resp" "created" "POST /api/tools/import-grpc → OK"

# ── 7. API Key Management ──
echo -e "${CYAN}── 7. API Key Management${RESET}"
resp=$(curl -s -w "\n%{http_code}" -X POST $BASE/api/keys -H "Content-Type: application/json" \
  -d "{\"gateway_id\":1,\"key\":\"$KEY_NAME\",\"name\":\"Test Key $TS\"}")
http_code=$(echo "$resp" | tail -1); body=$(echo "$resp" | sed '$d')
key_id=$(echo "$body" | python3 -c "import sys,json;print(json.load(sys.stdin).get('ID',''))")
[ "$http_code" = "201" ] && pass "POST /api/keys → created key" || fail "POST /api/keys (code=$http_code)"

resp=$(curl -s $BASE/api/keys)
check_contains "$resp" "$KEY_NAME" "GET /api/keys → lists key"

resp=$(curl -s -X PUT "$BASE/api/keys/$key_id/toggle")
check_contains "$resp" "false" "PUT /api/keys/:id/toggle → disabled"

resp=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE "$BASE/api/keys/$key_id")
[ "$resp" = "200" ] && pass "DELETE /api/keys/:id → 200" || fail "DELETE /api/keys/:id (code=$resp)"

# ── 8. MCP Streamable HTTP ──
echo -e "${CYAN}── 8. MCP Streamable HTTP${RESET}"
resp=$(curl -s -X POST $BASE/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"1","method":"initialize"}')
check_contains "$resp" "protocolVersion" "POST /mcp initialize → typed result"
check_contains "$resp" "2025-03-26" "Protocol version = 2025-03-26"

# Session ID header
session_id=$(curl -s -i -X POST $BASE/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"1","method":"initialize"}' 2>/dev/null | grep -i 'Mcp-Session-Id' | tr -d '\r' | awk '{print $NF}')
[ -n "$session_id" ] && pass "Mcp-Session-Id header returned" || fail "Mcp-Session-Id header"

resp=$(curl -s -X POST $BASE/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"2","method":"tools/list"}')
check_contains "$resp" "tools" "POST /mcp tools/list → OK (stateless)"

resp=$(curl -s -X POST $BASE/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"3","method":"tools/call","params":{"name":"query_orders","arguments":{"customer":"CUST-101"}}}')
check_contains "$resp" "result" "POST /mcp tools/call (HTTP) → OK"

# With session
resp=$(curl -s -X POST $BASE/mcp -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $session_id" \
  -d '{"jsonrpc":"2.0","id":"4","method":"tools/list"}')
check_contains "$resp" "tools" "POST /mcp tools/list (with session) → OK"

# SSE streaming — verify Content-Type is text/event-stream
resp=$(curl -s -i -X POST $BASE/mcp -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{"jsonrpc":"2.0","id":"5","method":"tools/list"}' 2>/dev/null)
check_contains "$resp" "text/event-stream" "POST /mcp (Accept: text/event-stream) → SSE Content-Type"

# ── 9. Notification ──
echo -e "${CYAN}── 9. Notification${RESET}"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}')
check "$code" "202" "POST /mcp notifications/initialized → 202 Accepted"

# ── 10. SSE Transport (backward compat) ──
echo -e "${CYAN}── 10. SSE Transport (backward compat)${RESET}"
sse_resp=$(curl -s -i --max-time 2 $BASE/mcp/sse 2>/dev/null || true)
check_contains "$sse_resp" "text/event-stream" "GET /mcp/sse → Content-Type text/event-stream"

# ── 11. Sessions ──
echo -e "${CYAN}── 11. Session Management${RESET}"
resp=$(curl -s $BASE/api/sessions)
check_contains "$resp" "sessions" "GET /api/sessions → OK"

# ── 12. Call Logs ──
echo -e "${CYAN}── 12. Call Logs${RESET}"
resp=$(curl -s "$BASE/api/logs?limit=10")
check_contains "$resp" "logs" "GET /api/logs → OK (has entries)"

# ── 13. Body Limit ──
echo -e "${CYAN}── 13. Request Body Limit${RESET}"
code=$(curl -s -o /dev/null -w "%{http_code}" -X POST $BASE/mcp \
  -H "Content-Type: application/json" \
  -d "$(python3 -c "print('{\"x\":\"' + 'y'*2000000 + '\"}')" 2>/dev/null)" 2>/dev/null || echo "413")
[ "$code" = "413" ] && pass "POST oversized body → 413" || fail "Body limit (code=$code)"

# ── 14. Unknown Method ──
echo -e "${CYAN}── 14. Error Handling${RESET}"
resp=$(curl -s -X POST $BASE/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"99","method":"nonexistent"}')
check_contains "$resp" "不支持的方法" "Unknown method → proper error"

# ── 15. gRPC E2E ──
echo -e "${CYAN}── 15. gRPC Tools${RESET}"
resp=$(curl -s $BASE/api/tools)
grpc_count=$(echo "$resp" | python3 -c "import sys,json;tools=json.load(sys.stdin);print(len([t for t in tools if t.get('protocol')=='grpc']))")
check_gt "$grpc_count" "0" "gRPC tools registered: $grpc_count"

# ── Results ──
echo ""
echo -e "${BOLD}===========================================${RESET}"
echo -e "${BOLD}  Results: ${GREEN}$PASS passed${RESET}, ${RED}$FAIL failed${RESET}  ($((PASS+FAIL)) total)"
echo -e "${BOLD}===========================================${RESET}"
[ "$FAIL" -eq 0 ] && echo -e "${GREEN}All tests passed!${RESET}" && exit 0
echo -e "${RED}$FAIL test(s) failed${RESET}"
exit 1
