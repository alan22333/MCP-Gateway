# MCP Nexus — AI 模型上下文协议多协议网关

[![CodeFactor](https://www.codefactor.io/repository/github/alan22333/mcp-gateway/badge)](https://www.codefactor.io/repository/github/alan22333/mcp-gateway)
[![CI](https://github.com/alan22333/mcp-nexus/actions/workflows/ci.yml/badge.svg)](https://github.com/alan22333/mcp-nexus/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

将你的 REST API 和 gRPC 服务一键暴露为 AI 可调用的 MCP 工具。支持 OpenAPI 文档导入和 .proto 文件解析。

---

## 为什么需要 MCP Nexus？

让 AI 调用你的后端服务，需要实现 [MCP 协议](https://spec.modelcontextprotocol.io/)——JSON-RPC 2.0、Streamable HTTP、SSE、工具发现、参数校验……每个服务都自己写一套 MCP Server，成本太高。

MCP Nexus 把这件事搬到网关层：

```text
AI 客户端 ← MCP 协议 → MCP Nexus 网关 ← HTTP/gRPC → 你的后端服务
```

你只需要打开管理后台，告诉网关你的 API 地址或上传 OpenAPI/.proto 文件，网关自动生成 MCP 工具定义。AI 客户端连上 `/mcp` 端点就能发现和调用所有工具。

---

## 快速开始

```bash
docker compose up -d
open http://localhost:8080
```

打开管理后台后三步走：

1. **添加工具** — 点击"导入工具"粘贴 OpenAPI 文档地址或 .proto 文件，也可以点"新建工具"手动填写后端 URL
2. **（可选）创建网关** — 默认已有一个 Default Gateway，你可以为不同团队/项目创建独立网关隔离工具集
3. **连接 AI 客户端** — 将 Claude / Codex / Cline 等指向 `http://localhost:8080/mcp`

---

## 功能

| 类别 | 特性 |
| ---- | ---- |
| **MCP 协议** | Streamable HTTP (MCP 2025) + SSE 向后兼容 |
| **多协议代理** | HTTP REST (resty) + gRPC (dynamicpb 运行时动态调用) |
| **工具导入** | OpenAPI 3.0 / Swagger 2.0 / .proto 文件一键解析 |
| **多租户** | Gateway 实体隔离工具集 + API Key 认证 |
| **流量控制** | 令牌桶限流 + 信号量并发控制 + gobreaker 熔断 |
| **缓存** | Redis / 内存双模式请求去重 + 写后失效 |
| **可观测性** | TraceID 全链路 + Prometheus /metrics + 结构化日志 |
| **配置** | config.yaml + 热更新 + MCP_ 环境变量覆盖 |

### MCP Streamable HTTP

实现了 MCP 2025 最新规范：单一 `POST /mcp` 端点处理所有 JSON-RPC 请求，根据 `Accept` header 自动返回 JSON 或 SSE 流，通过 `Mcp-Session-Id` header 管理会话。同时保留 `GET /mcp/sse` 端点向后兼容旧版客户端。

### 多协议代理

同一个网关同时支持 HTTP 和 gRPC 后端，根据工具类型自动选择代理方式：

- **HTTP** — 通过 resty 转发，支持 GET/POST/PUT/DELETE，自动将 AI 传参映射到 query string 或 request body
- **gRPC** — 通过 `dynamicpb` 运行时动态构造 protobuf 消息并调用，**不需要预编译 .proto 或生成 stub 代码**。网关启动时解析 .proto 文件，注册 FileDescriptorSet，调用时通过 gRPC Reflection 发现方法，JSON 参数自动转换为 protobuf 消息

### 工具导入

不用手动填写参数 schema：粘贴 OpenAPI 文档 URL 自动解析所有端点，粘贴 .proto 内容自动解析所有 service method，支持预览模式，确认后批量导入。

### 流量保护

防止 AI 客户端失控循环调用打垮后端：

- **令牌桶限流** — 每 session 独立限流，默认 5 req/s + 10 burst
- **并发控制** — 信号量限制每 session 同时进行的调用数
- **熔断** — 后端连续失败 N 次后自动熔断，超时后半开探测，恢复后放行

### 可观测性

每个请求分配唯一 TraceID，贯穿所有结构化日志（zap JSON 格式）。Prometheus `/metrics` 暴露活跃 session 数、调用次数、延迟分布。管理后台可查看每次工具调用的详细记录。

---

## 配置参考

```yaml
server:
  port: 8080
  max_body_bytes: 1048576   # 请求体上限

database:
  driver: sqlite            # sqlite / mysql
  dsn: gateway.db

# 启动时自动注册的后端服务（可选，也可在管理后台手动添加）
backends:
  - name: my-api
    openapi_url: https://my-api.example.com/openapi.json
    base_url: https://my-api.example.com
  - name: my-grpc
    grpc_proto: |
      syntax = "proto3"; package orders;
      service OrderService { rpc GetOrder(GetOrderRequest) returns (Order); }
      message GetOrderRequest { string order_id = 1; }
      message Order { string order_id = 1; string customer = 2; }
    grpc_addr: grpc.internal:50051

cache:
  enabled: true
  redis_addr: ""            # 留空使用内存缓存
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
  enabled: false            # 设为 true 启用 API Key 认证
```

所有配置项支持 `MCP_` 前缀环境变量覆盖，例如 `MCP_SERVER_PORT=9090`。

---

## 项目结构

```text
├── cmd/server/         # 网关入口
├── internal/
│   ├── handler/        # HTTP handlers（MCP / Gateway / Tool / Import / Session）
│   ├── service/        # 核心业务（多协议调度、参数校验、缓存）
│   ├── proxy/          # HTTP 代理 + gRPC 动态代理 + 熔断
│   ├── repository/     # 数据访问层
│   ├── model/          # GORM 数据模型
│   ├── middleware/     # TraceID / Auth / BodyLimit
│   ├── config/         # Viper 配置 + 热更新
│   ├── cache/          # 缓存接口（内存/Redis）
│   └── metrics/        # Prometheus 指标
├── pkg/
│   ├── mcp/            # JSON-RPC 2.0 协议定义
│   ├── openapi/        # OpenAPI / Swagger 解析
│   ├── protobuf/       # .proto 解析 → MCP Tool
│   └── sse/            # SSE Writer
├── web/                # 管理后台
├── config.yaml         # 配置文件
├── dev/                # 开发工具（mock 后端、种子数据）
└── docs/               # 学习文档
```

---

## 技术栈

| 类别 | 选型 |
| ---- | ---- |
| Web 框架 | gin-gonic/gin |
| HTTP 代理 | go-resty/resty/v2 |
| gRPC 动态调用 | google.golang.org/grpc + dynamicpb |
| Proto 解析 | jhump/protoreflect |
| ORM + DB | gorm.io/gorm + SQLite |
| 配置 | spf13/viper (热更新 + 环境变量覆盖) |
| DI | google/wire (编译时) |
| 日志 | go.uber.org/zap (结构化 JSON) |
| 限流 | golang.org/x/time/rate |
| 熔断 | sony/gobreaker |
| 缓存 | go-redis/redis/v9 |
| 指标 | prometheus/client_golang |

---

## License

[LICENSE](LICENSE)
