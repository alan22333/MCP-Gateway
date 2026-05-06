#!/bin/bash
# MCP Gateway 端到端测试 —— 一键启动所有服务
#
# 启动顺序: mock-backend → seed → gateway → mock-client
# 退出时自动清理所有后台进程

set -e
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

cleanup() {
    echo ""
    echo "正在清理后台进程..."
    kill $MOCK_PID $GATEWAY_PID 2>/dev/null || true
    wait $MOCK_PID $GATEWAY_PID 2>/dev/null || true
    echo "已清理。"
}
trap cleanup EXIT INT TERM

echo "═══════════════════════════════════════════"
echo "  MCP Gateway 端到端测试启动脚本"
echo "═══════════════════════════════════════════"
echo ""

# 1. 编译所有组件
echo "[1/5] 编译组件..."
go build -o /tmp/mcp-gateway ./cmd/server/ &
go build -o /tmp/mock-backend ./cmd/mock-backend/ &
go build -o /tmp/mock-client ./cmd/mock-client/ &
go build -o /tmp/seed ./cmd/seed/ &
wait
echo "  ✓ 编译完成"

# 2. 启动模拟企业后端
echo "[2/5] 启动模拟企业后端 (port 9090)..."
/tmp/mock-backend &
MOCK_PID=$!
sleep 1
echo "  ✓ 模拟后端 PID=$MOCK_PID"

# 3. 写入种子数据
echo "[3/5] 写入工具配置种子数据..."
/tmp/seed
echo "  ✓ 种子数据就绪"

# 4. 启动 MCP Gateway
echo "[4/5] 启动 MCP Gateway (port 8080)..."
/tmp/mcp-gateway &
GATEWAY_PID=$!
sleep 1
echo "  ✓ Gateway PID=$GATEWAY_PID"

# 5. 运行 AI 客户端模拟器
echo "[5/5] 运行 AI 客户端模拟器..."
echo ""
/tmp/mock-client
CLIENT_EXIT=$?

echo ""
echo "═══════════════════════════════════════════"
echo "  测试结束 (退出码: $CLIENT_EXIT)"
echo "═══════════════════════════════════════════"
