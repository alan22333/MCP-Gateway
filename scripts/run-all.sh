#!/bin/bash
# MCP Gateway 端到端启动脚本 —— 一键启动所有服务
#
# 启动: bash scripts/run-all.sh
# 退出时自动清理所有后台进程

set -e
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

cleanup() {
    echo ""
    echo "Stopping services..."
    kill $MOCK_PID $GRPC_PID $GATEWAY_PID 2>/dev/null || true
    wait $MOCK_PID $GRPC_PID $GATEWAY_PID 2>/dev/null || true
    echo "All services stopped."
}
trap cleanup EXIT INT TERM

echo "==========================================="
echo "  MCP Gateway — E2E Startup"
echo "==========================================="
echo ""

# 1. Build all components
echo "[1/6] Building..."
go build -o /tmp/mcp-gateway ./cmd/server/ &
go build -o /tmp/mock-backend ./dev/mock-backend/ &
go build -o /tmp/mock-grpc-backend ./dev/mock-grpc-backend/ &
go build -o /tmp/mock-client ./dev/mock-client/ &
go build -o /tmp/seed ./dev/seed/ &
wait
echo "  Done"

# 2. Start HTTP mock backend
rm -f /tmp/gateway.db
echo "[2/6] Starting HTTP mock backend (port 9090)..."
/tmp/mock-backend > /dev/null 2>&1 &
MOCK_PID=$!
sleep 1
echo "  PID=$MOCK_PID"

# 3. Start gRPC mock backend
echo "[3/6] Starting gRPC mock backend (port 50052)..."
GRPC_PORT=50052 /tmp/mock-grpc-backend > /dev/null 2>&1 &
GRPC_PID=$!
sleep 1
echo "  PID=$GRPC_PID"

# 4. Seed data
echo "[4/6] Seeding database..."
/tmp/seed > /dev/null 2>&1
echo "  Done"

# 5. Start gateway
echo "[5/6] Starting MCP Gateway (port 8080)..."
/tmp/mcp-gateway > /dev/null 2>&1 &
GATEWAY_PID=$!
sleep 2
echo "  PID=$GATEWAY_PID"

# 6. Run mock client (SSE + Streamable HTTP)
echo "[6/6] Running mock client..."
echo ""
/tmp/mock-client
EXIT_CODE=$?

echo ""
echo "==========================================="
echo "  Done (exit: $EXIT_CODE)"
echo "==========================================="
