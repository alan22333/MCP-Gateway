package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alan22333/mcp-nexus/internal/cache"
	"github.com/alan22333/mcp-nexus/internal/metrics"
	"github.com/alan22333/mcp-nexus/internal/model"
	"github.com/alan22333/mcp-nexus/internal/proxy"
	"github.com/alan22333/mcp-nexus/pkg/mcp"

	"go.uber.org/zap"
)

// toolRepo McpService 需要的仓库方法子集（接口隔离原则）
type toolRepo interface {
	GetByToolName(gatewayID uint, name string) (*model.ApiTool, error)
	GetToolsByGateway(gatewayID uint) ([]model.ApiTool, error)
	CreateCallLog(log *model.CallLog) error
}

// McpService MCP 服务核心，实现 handler.RequestProcessor 接口
type McpService struct {
	repo      toolRepo
	httpProxy *proxy.HttpProxy
	grpcProxy *proxy.GrpcProxy
	cbManager *proxy.CircuitBreakerManager
	cache     cache.ToolCache
	cacheTTL  time.Duration
	logger    *zap.Logger
}

// NewMcpService 创建 MCP 服务实例，注入 HTTP 代理、gRPC 代理、熔断器和缓存实现。
func NewMcpService(repo toolRepo, httpP *proxy.HttpProxy, grpcP *proxy.GrpcProxy, cb *proxy.CircuitBreakerManager, c cache.ToolCache, cacheTTL time.Duration, logger *zap.Logger) *McpService {
	return &McpService{repo: repo, httpProxy: httpP, grpcProxy: grpcP, cbManager: cb, cache: c, cacheTTL: cacheTTL, logger: logger}
}

// Process 是 MCP JSON-RPC 请求的统一入口，根据 method 分发到不同处理逻辑。
func (s *McpService) Process(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(gatewayID, req)
	case "tools/call":
		return s.handleToolsCall(ctx, gatewayID, req, "MCP")
	case "notifications/initialized":
		s.handleInitialized(req)
		return nil
	default:
		s.logger.Warn("不支持的方法", zap.String("method", req.Method))
		return mcp.NewError(req.ID, mcp.ErrCodeMethod, "不支持的方法: "+req.Method)
	}
}

// CallTool 供 HTTP handler 直接调用工具，返回底层代理响应和 MCP 响应。
// 内部复用 doToolsCall，避免代码重复。
func (s *McpService) CallTool(ctx context.Context, gatewayID uint, toolName string, args json.RawMessage, caller string) (*proxy.ProxyResponse, *mcp.RPCResponse) {
	return s.doToolsCall(ctx, gatewayID, nil, mcp.CallToolParams{Name: toolName, Arguments: args}, caller)
}

// ====== 私有方法 ======

// handleInitialize 返回 MCP 握手所需的基础协议信息和服务能力声明（typed result）。
func (s *McpService) handleInitialize(req *mcp.RPCRequest) *mcp.RPCResponse {
	result := &mcp.InitializeResult{
		ProtocolVersion: mcp.ProtocolVersion,
		ServerInfo:      mcp.ServerInfo{Name: "github.com/alan22333/mcp-nexus", Version: "2.0.0"},
		Capabilities: mcp.ServerCapabilities{
			Tools: &mcp.ToolsCapability{ListChanged: false},
		},
	}
	s.logger.Info("MCP 握手完成", zap.String("protocol", mcp.ProtocolVersion))
	return mcp.NewSuccess(req.ID, result)
}

// handleInitialized 处理客户端 initialized 通知（Streamable HTTP 握手第二步）
func (s *McpService) handleInitialized(req *mcp.RPCRequest) {
	s.logger.Info("客户端初始化完成通知")
}

// handleToolsList 查询当前网关下已启用的工具，并转换成 MCP 工具列表。
func (s *McpService) handleToolsList(gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse {
	tools, err := s.repo.GetToolsByGateway(gatewayID)
	if err != nil {
		s.logger.Error("查询工具列表失败", zap.Error(err))
		return mcp.NewError(req.ID, mcp.ErrCodeInternal, "查询工具列表失败")
	}
	mcpTools := make([]mcp.Tool, 0, len(tools))
	for _, t := range tools {
		mcpTools = append(mcpTools, apiToolToMCPTool(&t))
	}
	s.logger.Info("返回工具列表", zap.Int("count", len(mcpTools)), zap.Uint("gateway_id", gatewayID))
	return mcp.NewSuccess(req.ID, &mcp.ToolsListResult{Tools: mcpTools})
}

// handleToolsCall 解析 MCP 调用参数，并把执行流程交给 doToolsCall。
func (s *McpService) handleToolsCall(ctx context.Context, gatewayID uint, req *mcp.RPCRequest, caller string) *mcp.RPCResponse {
	var params mcp.CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewError(req.ID, mcp.ErrCodeInvalid, "参数解析失败: "+err.Error())
	}
	if params.Name == "" {
		return mcp.NewError(req.ID, mcp.ErrCodeInvalid, "缺少工具名称")
	}
	_, rpcResp := s.doToolsCall(ctx, gatewayID, req.ID, params, caller)
	return rpcResp
}

// doToolsCall 执行完整的工具调用流程：查工具 → 校验参数 → 缓存检查 → 代理转发 → 写缓存 → 组装响应。
// 返回 proxy 原始响应和 MCP 响应，供 handler 层灵活使用。
func (s *McpService) doToolsCall(ctx context.Context, gatewayID uint, id interface{}, params mcp.CallToolParams, caller string) (*proxy.ProxyResponse, *mcp.RPCResponse) {
	tool, err := s.repo.GetByToolName(gatewayID, params.Name)
	if err != nil {
		s.logger.Warn("工具不存在", zap.String("tool", params.Name), zap.Uint("gateway_id", gatewayID))
		return nil, mcp.NewError(id, mcp.ErrCodeMethod, fmt.Sprintf("工具不存在: %s", params.Name))
	}

	// ── 1. 参数校验 ──
	if v := validateArgs(tool.InputSchema, params.Arguments); !v.Valid {
		s.logger.Warn("参数校验失败", zap.String("tool", params.Name), zap.Any("issues", v.Issues))
		metrics.RecordToolCall(params.Name, "validation_error", 0)
		msg, err := json.Marshal(map[string]interface{}{
			"error": "参数校验失败，请修正后重试", "tool": params.Name, "issues": v.Issues,
		})
		if err != nil {
			msg = []byte(`{"error":"参数校验失败"}`)
		}
		return nil, mcp.NewError(id, mcp.ErrCodeInvalid, string(msg))
	}

	// ── 2. 缓存检查 ──
	group := cache.CacheGroup(tool.BackendUrl)
	if entry, ok := s.cache.Get(ctx, group, params.Name, params.Arguments); ok {
		s.logger.Info("缓存命中", zap.String("tool", params.Name), zap.String("group", group))
		metrics.RecordToolCall(params.Name, "cache_hit", 0)
		return nil, mcp.NewSuccess(id, &mcp.CallToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: string(entry.Result)}},
		})
	}
	

	// ── 3. 代理转发 ──
	s.logger.Info("代理请求", zap.String("tool", params.Name),
		zap.String("protocol", tool.Protocol),
		zap.String("url", tool.BackendUrl), zap.String("group", group))

	start := time.Now()
	var proxyResp *proxy.ProxyResponse

	if tool.Protocol == "grpc" {
		proxyResp, err = s.grpcProxy.ForwardWithCB(ctx, s.cbManager, group, &proxy.GrpcRequest{
			Addr: tool.BackendUrl, Method: tool.HttpMethod, Args: params.Arguments,
		})
	} else {
		proxyResp, err = s.httpProxy.ForwardWithCB(ctx, s.cbManager, group, &proxy.ProxyRequest{
			Method: tool.HttpMethod, URL: tool.BackendUrl, Args: params.Arguments,
		})
	}
	elapsed := time.Since(start).Seconds()
	latency := int64(elapsed * 1000)
	s.logCall(params.Name, string(params.Arguments), proxyResp, err, latency, caller)

	if err != nil {
		s.logger.Error("后端请求失败", zap.Error(err),
			zap.String("tool", params.Name), zap.String("cb_state", s.cbManager.State(group)))
		metrics.RecordToolCall(params.Name, "backend_error", elapsed)
		metrics.SetCircuitBreakerState(group, s.cbManager.State(group))
		return proxyResp, mcp.NewError(id, mcp.ErrCodeInternal, "后端请求失败: "+err.Error())
	}

	metrics.RecordToolCall(params.Name, "success", elapsed)
	metrics.SetCircuitBreakerState(group, s.cbManager.State(group))

	// ── 4. 缓存处理 ──
	if tool.HttpMethod == "GET" && proxyResp.StatusCode == 200 {
		s.cache.Set(ctx, group, params.Name, params.Arguments, json.RawMessage(proxyResp.Body), s.cacheTTL)
	} else if tool.HttpMethod != "GET" && proxyResp.StatusCode >= 200 && proxyResp.StatusCode < 300 {
		s.cache.InvalidateGroup(ctx, group)
		s.logger.Info("写操作后已清除同组缓存", zap.String("tool", params.Name), zap.String("group", group))
	}

	callResult := formatCallResult(proxyResp.Body)
	return proxyResp, mcp.NewSuccess(id, callResult)
}

// formatCallResult 将后端响应的 JSON body 格式化为 MCP CallToolResult。
// 如果 body 不是合法 JSON，则作为纯文本返回。
func formatCallResult(body json.RawMessage) *mcp.CallToolResult {
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		// 非 JSON 响应 → 当作纯文本
		return &mcp.CallToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: string(body)}},
		}
	}
	contentText, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: string(body)}},
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: string(contentText)}},
	}
}

// logCall 记录一次工具调用日志到数据库
func (s *McpService) logCall(toolName, reqArgs string, proxyResp *proxy.ProxyResponse, err error, latency int64, caller string) {
	statusCode := 0
	responseBody := ""
	if proxyResp != nil {
		statusCode = proxyResp.StatusCode
		responseBody = string(proxyResp.Body)
	}
	log := &model.CallLog{
		ToolName:     toolName,
		RequestArgs:  reqArgs,
		ResponseBody: responseBody,
		StatusCode:   statusCode,
		LatencyMs:    latency,
		Caller:       caller,
	}
	if err != nil {
		log.ErrorMsg = err.Error()
	}
	if createErr := s.repo.CreateCallLog(log); createErr != nil {
		s.logger.Warn("写入调用日志失败", zap.Error(createErr))
	}
}

// safeString 安全地从 interface{} 中提取 string，失败返回默认值
func safeString(v interface{}, defaultVal string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return defaultVal
}

// apiToolToMCPTool 将内部模型 ApiTool 转换为 MCP 协议定义的 Tool
func apiToolToMCPTool(t *model.ApiTool) mcp.Tool {
	tool := mcp.Tool{
		Name:        t.ToolName,
		Description: t.Description,
		InputSchema: &mcp.JSONSchema{
			Type:       safeString(t.InputSchema["type"], "object"),
			Properties: make(map[string]interface{}),
		},
	}
	if props, ok := t.InputSchema["properties"]; ok {
		tool.InputSchema.Properties = props.(map[string]interface{})
	}
	if req, ok := t.InputSchema["required"]; ok {
		if reqList, ok := req.([]interface{}); ok {
			for _, r := range reqList {
				tool.InputSchema.Required = append(tool.InputSchema.Required, r.(string))
			}
		}
	}
	return tool
}
