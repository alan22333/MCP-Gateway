# MCP Gateway — AI 模型上下文协议网关平台

一个基于 Go 语言开发的 **MCP (Model Context Protocol) 网关中间件**。它的核心作用是将企业内部的 RESTful HTTP API 包装为符合 MCP 协议（JSON-RPC 2.0）的 "Tools"，通过 SSE 长连接供 AI 大模型调用。

## 能力总览

| 类别 | 特性 |
|------|------|
| **MCP 协议** | JSON-RPC 2.0 完整实现 (initialize / tools/list / tools/call)，SSE 全双工传输 |
| **HTTP 代理** | resty 代理层，支持 GET/POST/路径参数替换，全链路超时，TraceID 透传 |
| **多租户** | Gateway 实体抽象，每个网关有独立的工具集和 API Key 认证策略 |
| **AI 专属** | 参数校验防幻觉、请求去重缓存 (Redis + 内存)、写后缓存失效 |
| **流量控制** | 令牌桶限流 (per-session)、gobreaker 熔断 (按 backend 隔离) |
| **可观测性** | TraceID 全链路、Prometheus /metrics (Counter/Histogram/Gauge) |
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

# 5. 运行 AI 客户端模拟测试 (多网关隔离)
go run cmd/mock-client/main.go
```

或一键启动：

```bash
bash scripts/run-all.sh
```

## 架构

```
AI 大模型 ←── SSE ──→ MCP Gateway ── HTTP 代理 ──→ 企业后端 API
                        │
                        ├── Gateway 1 (订单服务): 3 个工具, API Key 认证
                        ├── Gateway 2 (客户仓库): 4 个工具, 公开
                        └── Default Gateway:      7 个工具, 公开
```

```
目录结构:
├── cmd/
│   ├── server/main.go        # 网关入口
│   ├── mock-backend/main.go   # 模拟企业后端
│   ├── mock-client/main.go    # AI 客户端模拟器
│   └── seed/main.go           # 种子数据
├── internal/
│   ├── config/                # viper 配置
│   ├── model/                 # GORM 模型
│   ├── repository/            # 数据访问层
│   ├── service/               # 核心业务逻辑
│   ├── handler/               # HTTP handlers + SSE
│   ├── proxy/                 # HTTP 代理 + 熔断
│   ├── middleware/             # TraceID + Auth
│   ├── cache/                 # Redis/内存缓存
│   └── metrics/               # Prometheus 指标
├── pkg/
│   ├── mcp/                   # MCP/JSON-RPC 协议
│   ├── openapi/               # OpenAPI/Swagger 解析
│   └── sse/                   # SSE 工具
├── web/index.html             # 管理后台前端
└── config.yaml                # 配置文件
```

## 更新历程

### v2.0 — 多租户网关平台 (2026-05-17)
- Gateway 实体抽象，工具按网关分组隔离
- ApiKey 绑定 Gateway，per-gateway 认证策略
- 复合唯一索引 (gateway_id, tool_name)
- 启动时 EnsureDefaultGateway 自动迁移
- 前端网关选择器 + 按网关过滤

### v1.9 — OpenAPI 导入增强 (2026-05-16)
- Swagger 2.0 兼容 (openapi2conv)
- URL 远程抓取 + servers 自动检测
- 预览 + 选择性导入

### v1.8 — 可观测性 (2026-05-16)
- TraceID 中间件 + proxy 透传
- Prometheus /metrics 端点
- Counter/Histogram/Gauge 三种指标类型

### v1.7 — API Key 认证 (2026-05-16)
- 可插拔认证中间件
- Header/Query 双通道取 Key
- 豁免路径 + 默认演示密钥

### v1.6 — 流量控制 (2026-05-15)
- 令牌桶限流 (per-session)
- gobreaker 熔断 (按 URL group 隔离)
- 5xx 视为失败触发熔断

### v1.5 — AI 专属特性 (2026-05-15)
- 参数 Schema 校验防幻觉
- Redis + 内存双模式请求去重缓存
- 缓存分组 + 写后失效

### v1.4 — Go 工程底座 (2026-05-15)
- Context 全链路超时
- http.Server.Shutdown 优雅启停

### v1.3 — 管理后台 (2026-05-01)
- 工具 CRUD + 调用测试 + 会话监控

### v1.0 — 基础网关 (2026-04-30)
- MCP SSE 长连接
- JSON-RPC 2.0 完整生命周期
- HTTP 代理转发
- SQLite + GORM 存储

## 技术栈

| 类别 | 选型 |
|------|------|
| Web 框架 | gin-gonic/gin |
| HTTP 代理 | go-resty/resty/v2 |
| ORM + DB | gorm.io/gorm + SQLite |
| 配置 | spf13/viper |
| 日志 | go.uber.org/zap |
| OpenAPI 解析 | getkin/kin-openapi |
| 限流 | golang.org/x/time/rate |
| 熔断 | sony/gobreaker |
| 缓存 | go-redis/redis/v9 |
| 指标 | prometheus/client_golang |
| UUID | google/uuid |
| YAML | gopkg.in/yaml.v3 |

## 配置参考

```yaml
server:
  port: 8080

database:
  driver: sqlite
  dsn: gateway.db

cache:
  enabled: true
  redis_addr: ""        # 为空则使用内存缓存
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
  enabled: false         # 演示模式默认关闭
  exempt_paths:
    - /
    - /metrics
    - /api/health
```

## License

MIT
