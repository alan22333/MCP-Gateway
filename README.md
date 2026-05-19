# MCP Gateway — AI 模型上下文协议网关平台

一个基于 Go 语言开发的 **MCP (Model Context Protocol) 网关中间件**。它的核心作用是将企业内部的 RESTful HTTP API 包装为符合 MCP 协议（JSON-RPC 2.0）的 "Tools"，通过 HTTP 供 AI 大模型调用。

[![CodeFactor](https://www.codefactor.io/repository/github/alan22333/mcp-gateway/badge)](https://www.codefactor.io/repository/github/alan22333/mcp-gateway)

## 能力总览

| 类别 | 特性 |
|------|------|
| **MCP 协议** | JSON-RPC 2.0 完整实现 (initialize / tools/list / tools/call)，**Streamable HTTP** + SSE 双传输 |
| **HTTP 代理** | resty 代理层，支持 GET/POST/路径参数替换，全链路超时，TraceID 透传 |
| **多租户** | Gateway 实体抽象，每个网关有独立的工具集和 API Key 认证策略 |
| **AI 专属** | 参数校验防幻觉、请求去重缓存 (Redis + 内存)、写后缓存失效 |
| **流量控制** | 令牌桶限流 (per-session)、信号量并发控制、gobreaker 熔断 (按 backend 隔离) |
| **请求防护** | 请求体大小限制 (http.MaxBytesReader)、API Key 认证中间件 (per-gateway) |
| **可观测性** | TraceID 全链路、Prometheus /metrics (Counter/Histogram/Gauge) |
| **配置** | Viper 加载 config.yaml + WatchConfig 热更新 (限流/认证开关实时生效) |
| **DI** | Google Wire 编译时依赖注入，自动解析依赖图 |
| **安全** | API Key 认证中间件 (per-gateway、可插拔、豁免路径) |
| **导入** | OpenAPI 3.0 + Swagger 2.0 一键导入，URL 远程抓取，预览勾选 |
| **管理后台** | 纯 HTML/JS 单页：网关管理、工具 CRUD、调用测试、日志查看 |

## 快速开始

```bash
# 1. 启动模拟企业后端 (订单/客户/库存 API)
go run cmd/mock-backend/main.go

# 2. 写入种子数据 (3 个网关 + 14 个工具)
go run cmd/seed/main.go

# 3. 启动网关
go run cmd/server/main.go

# 4. 打开管理后台
open http://localhost:8080

# 5. 运行 AI 客户端模拟测试 (SSE + Streamable HTTP 双传输)
go run cmd/mock-client/main.go
```

或一键启动：

```bash
bash scripts/run-all.sh
```

## 传输方式

| 端点 | 方法 | 传输协议 | 说明 |
|------|------|----------|------|
| **`/mcp`** | **POST** | **Streamable HTTP** (MCP 2025) | 统一端点，支持 JSON 直接响应 + SSE 流式响应，Mcp-Session-Id header 会话管理 |
| `/mcp/sse` | GET | SSE (旧版，保留兼容) | SSE 长连接，返回 session_id |
| `/mcp/message` | POST | SSE (旧版，保留兼容) | 通过 `?session_id=` 发送 JSON-RPC 请求 |

**Streamable HTTP 快速测试**：

```bash
# 无状态 JSON 响应
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":"1","method":"initialize"}'

# 带 session 的请求
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: <uuid-from-initialize>" \
  -d '{"jsonrpc":"2.0","id":"2","method":"tools/list"}'

# SSE 流式响应
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{"jsonrpc":"2.0","id":"3","method":"tools/list"}'
```

## 架构

```
AI 大模型 ←── HTTP ──→ MCP Gateway ── HTTP 代理 ──→ 企业后端 API
     │                      │
     │  POST /mcp           ├── Gateway 1 (订单服务): 3 个工具, API Key 认证
     │  (Streamable HTTP)   ├── Gateway 2 (客户仓库): 4 个工具, 公开
     │  Mcp-Session-Id      └── Default Gateway:      7 个工具, 公开
     │
     └── GET /mcp/sse (旧 SSE 传输，向后兼容)
```

```
目录结构:
├── cmd/
│   ├── server/main.go        # 网关入口 (Wire DI + RouterGroup)
│   ├── mock-backend/main.go   # 模拟企业后端
│   ├── mock-client/main.go    # AI 客户端模拟器 (双传输测试)
│   └── seed/main.go           # 种子数据 (3 网关 + 14 工具)
├── internal/
│   ├── config/                # viper 配置 + 热更新 + 测试
│   ├── model/                 # GORM 模型 (Gateway, ApiTool, ApiKey, CallLog)
│   ├── repository/            # 数据访问层 (CRUD + AutoMigrate)
│   ├── service/               # 核心业务逻辑 (MCP 握手/工具列表/调用)
│   ├── handler/               # HTTP handlers
│   │   ├── streamable_handler.go  # Streamable HTTP (POST /mcp)
│   │   ├── sse_handler.go         # 旧 SSE 传输 (GET /mcp/sse)
│   │   ├── session.go             # Session 管理器 (限流 + 并发 + TTL 清理)
│   │   └── ...                    # Gateway/Tool/Key/Import/Log handlers
│   ├── proxy/                 # HTTP 代理 + 熔断 (gobreaker)
│   ├── middleware/             # TraceID + Auth + BodyLimit (含测试)
│   ├── cache/                 # Redis/内存缓存 (分组失效)
│   └── metrics/               # Prometheus 指标
├── pkg/
│   ├── mcp/                   # MCP/JSON-RPC 协议 (InitializeResult, Notification)
│   ├── openapi/               # OpenAPI 3.0 + Swagger 2.0 解析
│   └── sse/                   # SSE Writer
├── web/index.html             # 管理后台前端
└── config.yaml                # 配置文件 (支持热更新)
```

## 更新历程

### v2.1 — Streamable HTTP 传输 + 工程化加固 (2026-05-19)
- **Streamable HTTP**：MCP 2025 规范统一端点 `POST /mcp`，支持 JSON 直接响应 + SSE 流式
- `Mcp-Session-Id` header 会话管理，支持有状态/无状态两种模式
- JSON-RPC Notification 支持 (`notifications/initialized` → 202 Accepted)
- 协议类型升级：`InitializeResult`/`ServerCapabilities` 类型化，协议版本 `2025-03-26`
- **Per-Session 并发控制**：channel 信号量限制单 session 并发调用数
- **请求体大小限制**：`http.MaxBytesReader` 中间件防 DoS
- **配置热更新**：Viper `WatchConfig()` + `OnConfigChange()` + `atomic.Value` 无锁共享
- **Wire 依赖注入**：编译时代码生成，Provider Set 分层
- **分组中间件栈**：`gin.RouterGroup` 按功能分组 (MCP/API/Public)，`gin.IRouter` 接口解耦
- **项目结构规范化**：`go.mod` 直接依赖修正、文件重命名、middleware/config 测试补全
- 旧 SSE 传输完整保留向后兼容

### v2.0 — 多租户网关平台
- Gateway 实体抽象，工具按网关分组隔离
- ApiKey 绑定 Gateway，per-gateway 认证策略
- 复合唯一索引 (gateway_id, tool_name)
- 启动时 EnsureDefaultGateway 自动迁移
- 前端网关选择器 + 按网关过滤

### v1.9 — OpenAPI 导入增强
- Swagger 2.0 兼容 (openapi2conv)
- URL 远程抓取 + servers 自动检测
- 预览 + 选择性导入

### v1.8 — 可观测性
- TraceID 中间件 + proxy 透传
- Prometheus /metrics 端点
- Counter/Histogram/Gauge 三种指标类型

### v1.7 — API Key 认证
- 可插拔认证中间件
- Header/Query 双通道取 Key
- 豁免路径 + 默认演示密钥

### v1.6 — 流量控制 
- 令牌桶限流 (per-session)
- gobreaker 熔断 (按 URL group 隔离)
- 5xx 视为失败触发熔断

### v1.5 — 校验与缓存
- 参数 Schema 校验防幻觉
- Redis + 内存双模式请求去重缓存
- 缓存分组 + 写后失效

### v1.4 — Go 工程底座
- Context 全链路超时
- http.Server.Shutdown 优雅启停

### v1.3 — 管理后台
- 工具 CRUD + 调用测试 + 会话监控

### v1.0 — 基础网关
- MCP SSE 长连接
- JSON-RPC 2.0 完整生命周期
- HTTP 代理转发
- SQLite + GORM 存储

## 技术栈

| 类别 | 选型 |
|------|------|
| Web 框架 | gin-gonic/gin |
| HTTP 代理 | go-resty/resty/v2 |
| DI 框架 | google/wire (编译时) |
| ORM + DB | gorm.io/gorm + SQLite |
| 配置 | spf13/viper (热更新) |
| 日志 | go.uber.org/zap |
| OpenAPI 解析 | getkin/kin-openapi |
| 限流 | golang.org/x/time/rate |
| 熔断 | sony/gobreaker |
| 缓存 | go-redis/redis/v9 |
| 指标 | prometheus/client_golang |
| UUID | google/uuid |

## 配置参考

```yaml
server:
  port: 8080              # 网关监听端口
  max_body_bytes: 1048576 # 请求体最大字节数 (默认 1MB)

database:
  driver: sqlite
  dsn: gateway.db

cache:
  enabled: true
  redis_addr: ""          # 为空则使用内存缓存
  ttl: 60

rate_limit:
  enabled: true
  requests_per_second: 5
  burst: 10

circuit_breaker:
  enabled: true
  max_failures: 5
  timeout: 30

auth:
  enabled: false          # 演示模式默认关闭
  exempt_paths:
    - /
    - /metrics
    - /api/health
```

## License

MIT
