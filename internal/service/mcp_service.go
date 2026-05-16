package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"mcp-gateway-go-demo/internal/cache"
	"mcp-gateway-go-demo/internal/metrics"
	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/proxy"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/pkg/mcp"

	"go.uber.org/zap"
)

// McpService MCP 服务核心，实现 handler.RequestProcessor 接口
type McpService struct {
	repo      *repository.ApiToolRepo
	proxy     *proxy.HttpProxy
	cbManager *proxy.CircuitBreakerManager
	cache     cache.ToolCache
	cacheTTL  time.Duration
	logger    *zap.Logger
}

func NewMcpService(repo *repository.ApiToolRepo, p *proxy.HttpProxy, cb *proxy.CircuitBreakerManager, c cache.ToolCache, cacheTTL time.Duration, logger *zap.Logger) *McpService {
	return &McpService{repo: repo, proxy: p, cbManager: cb, cache: c, cacheTTL: cacheTTL, logger: logger}
}

func (s *McpService) Process(ctx context.Context, req *mcp.RPCRequest) *mcp.RPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req, "MCP")
	default:
		s.logger.Warn("不支持的方法", zap.String("method", req.Method))
		return mcp.NewError(req.ID, mcp.ErrCodeMethod, "不支持的方法: "+req.Method)
	}
}

func (s *McpService) CallTool(ctx context.Context, toolName string, args json.RawMessage, caller string) (*proxy.ProxyResponse, *mcp.RPCResponse) {
	tool, err := s.repo.GetByToolName(toolName)
	if err != nil {
		return nil, mcp.NewError(nil, mcp.ErrCodeMethod, fmt.Sprintf("工具不存在: %s", toolName))
	}

	if v := validateArgs(tool.InputSchema, args); !v.Valid {
		s.logger.Warn("参数校验失败", zap.String("tool", toolName), zap.Any("issues", v.Issues))
		metrics.RecordToolCall(toolName, "validation_error", 0)
		msg, _ := json.Marshal(map[string]interface{}{
			"error":  "参数校验失败，请修正后重试",
			"tool":   toolName,
			"issues": v.Issues,
		})
		return nil, mcp.NewError(nil, mcp.ErrCodeInvalid, string(msg))
	}

	group := cache.CacheGroup(tool.BackendUrl)
	if entry, ok := s.cache.Get(ctx, group, toolName, args); ok {
		s.logger.Info("缓存命中", zap.String("tool", toolName), zap.String("group", group))
		metrics.RecordToolCall(toolName, "cache_hit", 0)
		callResult := &mcp.CallToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: string(entry.Result)}},
		}
		return &proxy.ProxyResponse{StatusCode: 200, Body: entry.Result}, mcp.NewSuccess(nil, callResult)
	}

	start := time.Now()
	proxyResp, err := s.proxy.ForwardWithCB(ctx, s.cbManager, group, &proxy.ProxyRequest{
		Method: tool.HttpMethod, URL: tool.BackendUrl, Args: args,
	})
	elapsed := time.Since(start).Seconds()
	latency := int64(elapsed * 1000)
	s.logCall(toolName, string(args), proxyResp, err, latency, caller)

	if err != nil {
		s.logger.Error("后端请求失败", zap.Error(err))
		metrics.RecordToolCall(toolName, "backend_error", elapsed)
		metrics.SetCircuitBreakerState(group, s.cbManager.State(group))
		return nil, mcp.NewError(nil, mcp.ErrCodeInternal, "后端请求失败: "+err.Error())
	}

	metrics.RecordToolCall(toolName, "success", elapsed)
	metrics.SetCircuitBreakerState(group, s.cbManager.State(group))

	if tool.HttpMethod == "GET" && proxyResp.StatusCode == 200 {
		s.cache.Set(ctx, group, toolName, args, json.RawMessage(proxyResp.Body), s.cacheTTL)
	} else if tool.HttpMethod != "GET" && proxyResp.StatusCode >= 200 && proxyResp.StatusCode < 300 {
		s.cache.InvalidateGroup(ctx, group)
	}

	var result interface{}
	if json.Unmarshal(proxyResp.Body, &result) != nil {
		result = string(proxyResp.Body)
	}
	contentText, _ := json.Marshal(result)
	callResult := &mcp.CallToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: string(contentText)}},
	}
	return proxyResp, mcp.NewSuccess(nil, callResult)
}

// ====== 私有方法 ======

func (s *McpService) handleInitialize(req *mcp.RPCRequest) *mcp.RPCResponse {
	result := map[string]interface{}{
		"protocolVersion": "0.1.0",
		"serverInfo":      map[string]string{"name": "mcp-gateway-go-demo", "version": "1.0.0"},
		"capabilities":    map[string]interface{}{"tools": map[string]bool{}},
	}
	s.logger.Info("MCP 握手完成")
	return mcp.NewSuccess(req.ID, result)
}

func (s *McpService) handleToolsList(req *mcp.RPCRequest) *mcp.RPCResponse {
	tools, err := s.repo.GetEnabled()
	if err != nil {
		s.logger.Error("查询工具列表失败", zap.Error(err))
		return mcp.NewError(req.ID, mcp.ErrCodeInternal, "查询工具列表失败")
	}
	mcpTools := make([]mcp.Tool, 0, len(tools))
	for _, t := range tools {
		mcpTools = append(mcpTools, apiToolToMCPTool(&t))
	}
	s.logger.Info("返回工具列表", zap.Int("count", len(mcpTools)))
	return mcp.NewSuccess(req.ID, &mcp.ToolsListResult{Tools: mcpTools})
}

func (s *McpService) handleToolsCall(ctx context.Context, req *mcp.RPCRequest, caller string) *mcp.RPCResponse {
	var params mcp.CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewError(req.ID, mcp.ErrCodeInvalid, "参数解析失败: "+err.Error())
	}
	if params.Name == "" {
		return mcp.NewError(req.ID, mcp.ErrCodeInvalid, "缺少工具名称")
	}
	return s.doToolsCall(ctx, req.ID, params, caller)
}

func (s *McpService) doToolsCall(ctx context.Context, id interface{}, params mcp.CallToolParams, caller string) *mcp.RPCResponse {
	tool, err := s.repo.GetByToolName(params.Name)
	if err != nil {
		s.logger.Warn("工具不存在", zap.String("tool", params.Name))
		return mcp.NewError(id, mcp.ErrCodeMethod, fmt.Sprintf("工具不存在: %s", params.Name))
	}

	// ── 1. 参数校验 ──
	if v := validateArgs(tool.InputSchema, params.Arguments); !v.Valid {
		s.logger.Warn("参数校验失败", zap.String("tool", params.Name), zap.Any("issues", v.Issues))
		metrics.RecordToolCall(params.Name, "validation_error", 0)
		msg, _ := json.Marshal(map[string]interface{}{
			"error": "参数校验失败，请修正后重试", "tool": params.Name, "issues": v.Issues,
		})
		return mcp.NewError(id, mcp.ErrCodeInvalid, string(msg))
	}

	// ── 2. 缓存检查 ──
	group := cache.CacheGroup(tool.BackendUrl)
	if entry, ok := s.cache.Get(ctx, group, params.Name, params.Arguments); ok {
		s.logger.Info("缓存命中", zap.String("tool", params.Name), zap.String("group", group))
		metrics.RecordToolCall(params.Name, "cache_hit", 0)
		return mcp.NewSuccess(id, &mcp.CallToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: string(entry.Result)}},
		})
	}

	// ── 3. 代理转发 ──
	s.logger.Info("代理请求", zap.String("tool", params.Name), zap.String("method", tool.HttpMethod),
		zap.String("url", tool.BackendUrl), zap.String("group", group))

	start := time.Now()
	proxyResp, err := s.proxy.ForwardWithCB(ctx, s.cbManager, group, &proxy.ProxyRequest{
		Method: tool.HttpMethod, URL: tool.BackendUrl, Args: params.Arguments,
	})
	elapsed := time.Since(start).Seconds()
	latency := int64(elapsed * 1000)
	s.logCall(params.Name, string(params.Arguments), proxyResp, err, latency, caller)

	if err != nil {
		s.logger.Error("后端请求失败", zap.Error(err),
			zap.String("tool", params.Name), zap.String("cb_state", s.cbManager.State(group)))
		metrics.RecordToolCall(params.Name, "backend_error", elapsed)
		metrics.SetCircuitBreakerState(group, s.cbManager.State(group))
		return mcp.NewError(id, mcp.ErrCodeInternal, "后端请求失败: "+err.Error())
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

	var result interface{}
	if json.Unmarshal(proxyResp.Body, &result) != nil {
		result = string(proxyResp.Body)
	}
	contentText, _ := json.Marshal(result)
	return mcp.NewSuccess(id, &mcp.CallToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: string(contentText)}},
	})
}

func (s *McpService) logCall(toolName, args string, resp *proxy.ProxyResponse, err error, latency int64, caller string) {
	log := &model.CallLog{ToolName: toolName, RequestArgs: args, LatencyMs: latency, Caller: caller}
	if resp != nil {
		log.ResponseBody = string(resp.Body)
		log.StatusCode = resp.StatusCode
	}
	if err != nil {
		log.ErrorMsg = err.Error()
	}
	go func() {
		if createErr := s.repo.CreateCallLog(log); createErr != nil {
			s.logger.Warn("写入调用日志失败", zap.Error(createErr))
		}
	}()
}

func apiToolToMCPTool(t *model.ApiTool) mcp.Tool {
	return mcp.Tool{
		Name:        t.ToolName,
		Description: t.Description,
		InputSchema: &mcp.JSONSchema{
			Type: "object", Properties: t.InputSchema, Required: []string{},
		},
	}
}
