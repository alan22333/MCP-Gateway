# 🎯 AI MCP Gateway (Go 版) 系统架构与开发设计蓝图

## 一、 项目定位与核心愿景
本项目是一个基于 Go 语言开发的 **AI MCP (Model Context Protocol) 网关中间件**。
它的核心作用是作为大模型（AI Agent）与企业内部传统业务接口（RESTful/HTTP API）之间的**翻译官和代理人**。
它将内部的 HTTP 接口包装为符合 MCP 协议（基于 JSON-RPC 2.0）的 "Tools"，并通过 SSE (Server-Sent Events) 长链接供 AI 调用。调用请求被网关拦截后，由网关发起真实的 HTTP 请求至后端业务系统，再将结果转换为 MCP 格式返回给 AI。

## 二、 技术栈约束规范
请在接下来的代码生成中，严格遵循以下技术选型：
*   **编程语言**：Go
*   **Web/路由框架**：`github.com/gin-gonic/gin` (高并发、生态好)
*   **HTTP 代理客户端**：`github.com/go-resty/resty/v2` (负责向后端发请求，需配置超时与重试)
*   **数据库与 ORM**：`gorm.io/gorm` + `gorm.io/driver/sqlite` (方便本地零配置启动，后期可无缝切 MySQL)
*   **OpenAPI 解析**：`github.com/getkin/kin-openapi/openapi3` (用于将 Swagger/OpenAPI 导入为 Tools)
*   **日志库**：`go.uber.org/zap` (结构化日志)
*   **配置文件**：`github.com/spf13/viper` (读取 `config.yaml`)

## 三、 标准工程目录结构 (Clean Architecture)
请按照以下结构初始化项目：
```text
mcp-gateway/
├── cmd/
│   └── server/
│       └── main.go               # 程序的组装与启动入口
├── internal/
│   ├── config/                   # viper 配置加载
│   ├── model/                    # 1. 数据库模型定义 2. JSON-RPC 协议结构体
│   ├── repository/               # GORM 数据库操作层 (DAO)
│   ├── service/                  # 核心业务逻辑 (协议转换、Tool管理)
│   ├── handler/                  # API 路由与控制器 (Gin handlers，重点是 SSE)
│   └── proxy/                    # HTTP 代理层 (封装 resty 发起业务请求)
├── pkg/
│   ├── mcp/                      # MCP/JSON-RPC 2.0 基础协议封装
│   ├── openapi/                  # Swagger 解析工具
│   └── sse/                      # SSE 流处理工具
├── config.yaml                   # 配置文件
├── go.mod
└── go.sum
```

## 四、 核心数据模型设计 (Domain Models)

### 1. 数据库表结构设计 (GORM Entity)
需要在 `internal/model` 中定义以下表结构：
*   **`ApiTool` (工具配置表)**：记录大模型能看到的 Tool 与实际 HTTP API 的映射关系。
    *   `ID` (uint, PK)
    *   `ToolName` (string, uniq) - 提供给 AI 的工具名称 (如: `get_order_info`)
    *   `Description` (string) - 工具的功能描述 (非常重要，AI 靠这个判断)
    *   `InputSchema` (json) - JSON Schema 格式，描述工具所需参数
    *   `BackendUrl` (string) - 真实被调用的后端地址 (如: `http://api.internal/order`)
    *   `HttpMethod` (string) - `GET`, `POST` 等

### 2. MCP 协议结构体设计 (JSON-RPC 2.0)
在 `pkg/mcp` 包下，严格定义以下结构体以实现序列化/反序列化：
```go
// 基础 JSON-RPC 请求
type RPCRequest struct {
    JSONRPC string          `json:"jsonrpc"` // 固定 "2.0"
    ID      interface{}     `json:"id"`      // 字符串或数字
    Method  string          `json:"method"`  // "initialize", "tools/list", "tools/call"
    Params  json.RawMessage `json:"params"`  // 延迟解析的动态参数
}

// 基础 JSON-RPC 响应
type RPCResponse struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      interface{} `json:"id"`
    Result  interface{} `json:"result,omitempty"`
    Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}
```

## 五、 核心业务工作流定义 (Workflows)

请在生成 `internal/handler` 和 `internal/service` 代码时，实现以下三个核心流程：

### 流程一：建立 SSE 监听与消息循环 (Session Management)
**目标**：大模型连接网关。
**实现规范**：
1. 在 Gin 路由中暴露 `GET /mcp/sse`。
2. 设置 Header: `Content-Type: text/event-stream`。
3. 获取 `c.Writer.(http.Flusher)`。
4. 保持连接不阻塞，通过 `select` 监听 Channel 收取消息，收到结果后使用 `fmt.Fprintf(c.Writer, "data: %s\n\n", jsonStr)` 并立刻调用 `Flusher.Flush()` 发送。

### 流程二：处理 `tools/list` 指令 (能力暴露)
**目标**：AI 询问网关“你有哪些工具？”。
**实现规范**：
1. 拦截到 `Method == "tools/list"` 的 `RPCRequest`。
2. 从 `repository` 层查询数据库中的 `ApiTool` 列表。
3. 将 `ApiTool` 列表转换为符合 MCP 规范的 Tool 结构。
4. 包装成 `RPCResponse`，通过流程一的 SSE 连接返回给大模型。

### 流程三：处理 `tools/call` 指令 (协议转换与动态代理)
**目标**：AI 决定使用某个工具，并传入了参数，网关需代为发起请求。
**实现规范**：
1. 拦截到 `Method == "tools/call"`，解析 `Params` 提取出 `name` (工具名) 和 `arguments` (AI给出的参数 JSON)。
2. 去数据库查询对应的 `ApiTool`，获取真实的 `BackendUrl` 和 `HttpMethod`。
3. 调用 `internal/proxy` 层：使用 `go-resty` 初始化 HTTP 请求。
    * 如果是 GET 请求，将 `arguments` 拼装为 Query 参数。
    * 如果是 POST 请求，将 `arguments` 作为 JSON Body。
4. **代理发起真实的 HTTP 业务请求**。
5. 拿到后端返回的 HTTP 响应内容（比如是一段 JSON），将其作为 `Result` 填入 `RPCResponse`。
6. 将 `RPCResponse` 通过 SSE 推送给 AI 大模型。

## 六、 AI Agent 生成代码的策略建议与约束
作为 AI 开发助手，请按照以下顺序**分步骤**输出/编写代码，并在每完成一步后向人类确认：
*   **Step 1**: 生成 `go.mod` 并在 `internal/model` 和 `pkg/mcp` 下编写所有基础的数据结构体 (DTO/DAO)。
*   **Step 2**: 编写 `repository` 层的 GORM 操作逻辑，实现 `ApiTool` 的增删查改。
*   **Step 3**: 编写 `pkg/sse` 和 `handler`，跑通 Gin 的 SSE 长链接。
*   **Step 4**: 编写 `proxy` 层与 `service` 层，实现接收到 `tools/call` 时的真实 HTTP 转发逻辑。
*   **Step 5**: 编写 `main.go` 组装依赖，使用 `sqlite` 完成整个项目的启动测试。

代码规范约束：
1. 所有外部输入必须有基本的合法性校验。
2. `proxy` 发起后端请求时，必须设置 10 秒超时。
3. 发生任何 Error（如连不上数据库、后端接口超时），不能 Panic，必须封装成 `RPCError` 格式返回给大模型。
4. 要有详细的中文注释，方便我学习这个项目精髓。
5. 不要过度设计，用最合适的方式的解决问题。
6. 要有丰富的测试和配套文档。
