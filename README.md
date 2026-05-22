# MCP Nexus — AI 模型上下文协议多协议网关

[![CodeFactor](https://www.codefactor.io/repository/github/alan22333/mcp-gateway/badge)](https://www.codefactor.io/repository/github/alan22333/mcp-gateway)
[![CI](https://github.com/alan22333/mcp-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/alan22333/mcp-gateway/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

将你的 REST API 和 gRPC 服务一键暴露为 AI 可调用的 MCP 工具。支持 OpenAPI 文档导入和 .proto 文件解析。

---

## 快速开始

### Docker

```bash
docker compose up -d
open http://localhost:8080
```

### Go 直接运行

```bash
go run ./cmd/server/
# 打开 http://localhost:8080
```

### 配置你的第一个工具

**方式 1：管理后台导入**（推荐）

打开 `http://localhost:8080` → "导入工具" → 粘贴 OpenAPI 文档 URL → 预览 → 确认导入

```bash
# 示例：导入一个有 OpenAPI 文档的后端
curl -X POST http://localhost:8080/api/tools/import \
  -H "Content-Type: application/json" \
  -d '{"url":"https://petstore.swagger.io/v2/swagger.json","gateway_id":1}'
```

**方式 2：config.yaml 预配置**

```yaml
# config.yaml
backends:
  - name: my-api
    openapi_url: https://my-company.com/openapi.json
    base_url: https://my-company.com/api

  - name: my-grpc
    grpc_proto: |
      syntax = "proto3"; package orders;
      service OrderService { rpc GetOrder(GetOrderRequest) returns (Order); }
      message GetOrderRequest { string order_id = 1; }
      message Order { string order_id = 1; string customer = 2; }
    grpc_addr: grpc.internal:50051
```

**方式 3：手动创建**

```bash
curl -X POST http://localhost:8080/api/tools \
  -H "Content-Type: application/json" \
  -d '{"gateway_id":1,"tool_name":"get_weather","description":"查询天气","backend_url":"https://api.weather.com/v1/current","http_method":"GET","protocol":"http"}'
```

### 连接 AI 客户端

设置你的 MCP 客户端（Claude Desktop / Continue / Cline 等）指向：

```
http://localhost:8080/mcp
```

客户端会通过 MCP 协议自动发现你注册的所有工具。

---

## 核心能力

| 类别 | 特性 |
|------|------|
| **MCP 协议** | Streamable HTTP (MCP 2025) + SSE 向后兼容 |
| **多协议代理** | HTTP REST (resty) + gRPC (dynamicpb 运行时动态调用) |
| **工具导入** | OpenAPI 3.0 / Swagger 2.0 / .proto 文件一键解析 |
| **多租户** | Gateway 实体隔离工具集 + API Key 认证 |
| **流量控制** | 令牌桶限流 + 信号量并发控制 + gobreaker 熔断 |
| **缓存** | Redis / 内存双模式请求去重 + 写后失效 |
| **可观测性** | TraceID 全链路 + Prometheus /metrics + 结构化日志 |
| **配置** | config.yaml + 热更新 + MCP_ 环境变量覆盖 |

---

## 连接你的后端

### HTTP API

```bash
# 从 OpenAPI 文档导入
curl -X POST "http://localhost:8080/api/tools/import?preview=true" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://your-api.com/openapi.json","gateway_id":1}'

# 或手动创建
curl -X POST http://localhost:8080/api/tools \
  -H "Content-Type: application/json" \
  -d '{"gateway_id":1,"tool_name":"my_tool","description":"...","backend_url":"https://your-api.com/endpoint","http_method":"GET","protocol":"http"}'
```

AI 客户端调用时，网关自动转发：`GET https://your-api.com/endpoint?param=value`

### gRPC 服务

```bash
# 从 .proto 文件导入
curl -X POST http://localhost:8080/api/tools/import-grpc \
  -H "Content-Type: application/json" \
  -d '{"proto_content":"syntax = \"proto3\"...","addr":"your-grpc:50051","gateway_id":1}'
```

网关通过 `dynamicpb` 在运行时动态构造 protobuf 消息并调用 gRPC 方法——**不需要你预编译 .proto 或生成 stub 代码**。

---

## 项目结构

```
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

## 配置参考

所有配置项支持 `MCP_` 前缀环境变量覆盖（Docker 友好）：

```yaml
server:
  port: 8080
  max_body_bytes: 1048576

database:
  driver: sqlite
  dsn: gateway.db

backends:             # 预注册你的后端服务（启动时自动导入）
  - name: my-api
    openapi_url: https://my-api.example.com/openapi.json
    base_url: https://my-api.example.com

cache:                # Redis 可选，留空使用内存缓存
  enabled: true
  redis_addr: ""

rate_limit:
  enabled: true
  requests_per_second: 5
  burst: 10

circuit_breaker:
  enabled: true
  max_failures: 5
  timeout: 30

auth:
  enabled: false      # 演示模式关闭认证
```

完整环境变量见 [.env.example](.env.example)。

---

## 技术栈

| 类别 | 选型 |
|------|------|
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
| 缓存 | go-redis/redis/v9 (可选) |
| 指标 | prometheus/client_golang |

---

## License

[LICENSE](LICENSE)
