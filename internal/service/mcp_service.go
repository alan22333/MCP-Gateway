package service

import (
	"encoding/json"
	"fmt"
	"time"

	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/proxy"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/pkg/mcp"

	"go.uber.org/zap"
)

// McpService MCP 服务核心，实现 handler.RequestProcessor 接口
type McpService struct {
	repo   *repository.ApiToolRepo
	proxy  *proxy.HttpProxy
	logger *zap.Logger
}

func NewMcpService(repo *repository.ApiToolRepo, proxy *proxy.HttpProxy, logger *zap.Logger) *McpService {
	return &McpService{repo: repo, proxy: proxy, logger: logger}
}

// Process 处理 JSON-RPC 请求的入口，根据 method 分发到不同处理逻辑
func (s *McpService) Process(req *mcp.RPCRequest) *mcp.RPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req, "MCP")
	default:
		s.logger.Warn("不支持的方法", zap.String("method", req.Method))
		return mcp.NewError(req.ID, mcp.ErrCodeMethod, "不支持的方法: "+req.Method)
	}
}

// CallTool 供外部（handler）直接调用，返回代理响应 + MCP 响应
func (s *McpService) CallTool(toolName string, args json.RawMessage, caller string) (*proxy.ProxyResponse, *mcp.RPCResponse) {
	tool, err := s.repo.GetByToolName(toolName)
	if err != nil {
		return nil, mcp.NewError(nil, mcp.ErrCodeMethod, fmt.Sprintf("工具不存在: %s", toolName))
	}

	start := time.Now()
	proxyResp, err := s.proxy.Forward(&proxy.ProxyRequest{
		Method: tool.HttpMethod,
		URL:    tool.BackendUrl,
		Args:   args,
	})
	latency := time.Since(start).Milliseconds()

	// 记录调用日志
	s.logCall(toolName, string(args), proxyResp, err, latency, caller)

	if err != nil {
		s.logger.Error("后端请求失败", zap.Error(err))
		return nil, mcp.NewError(nil, mcp.ErrCodeInternal, "后端请求失败: "+err.Error())
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
		"serverInfo": map[string]string{
			"name":    "mcp-gateway-go-demo",
			"version": "1.0.0",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]bool{},
		},
	}
	s.logger.Info("MCP 握手完成")
	return mcp.NewSuccess(req.ID, result)
}

// handleToolsList 只返回已启用的工具
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

func (s *McpService) handleToolsCall(req *mcp.RPCRequest, caller string) *mcp.RPCResponse {
	var params mcp.CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewError(req.ID, mcp.ErrCodeInvalid, "参数解析失败: "+err.Error())
	}
	if params.Name == "" {
		return mcp.NewError(req.ID, mcp.ErrCodeInvalid, "缺少工具名称")
	}
	return s.doToolsCall(req.ID, params, caller)
}

func (s *McpService) doToolsCall(id interface{}, params mcp.CallToolParams, caller string) *mcp.RPCResponse {
	tool, err := s.repo.GetByToolName(params.Name)
	if err != nil {
		s.logger.Warn("工具不存在", zap.String("tool", params.Name))
		return mcp.NewError(id, mcp.ErrCodeMethod, fmt.Sprintf("工具不存在: %s", params.Name))
	}

	s.logger.Info("代理请求", zap.String("tool", params.Name), zap.String("method", tool.HttpMethod), zap.String("url", tool.BackendUrl))

	start := time.Now()
	proxyResp, err := s.proxy.Forward(&proxy.ProxyRequest{
		Method: tool.HttpMethod,
		URL:    tool.BackendUrl,
		Args:   params.Arguments,
	})
	latency := time.Since(start).Milliseconds()

	// 记录调用日志
	s.logCall(params.Name, string(params.Arguments), proxyResp, err, latency, caller)

	if err != nil {
		s.logger.Error("后端请求失败", zap.Error(err))
		return mcp.NewError(id, mcp.ErrCodeInternal, "后端请求失败: "+err.Error())
	}

	var result interface{}
	if json.Unmarshal(proxyResp.Body, &result) != nil {
		result = string(proxyResp.Body)
	}
	contentText, _ := json.Marshal(result)

	callResult := &mcp.CallToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: string(contentText)}},
	}
	return mcp.NewSuccess(id, callResult)
}

// logCall 记录调用日志到数据库，异步写，不影响主流程
func (s *McpService) logCall(toolName, args string, resp *proxy.ProxyResponse, err error, latency int64, caller string) {
	log := &model.CallLog{
		ToolName:  toolName,
		RequestArgs: args,
		LatencyMs: latency,
		Caller:    caller,
	}
	if resp != nil {
		log.ResponseBody = string(resp.Body)
		log.StatusCode = resp.StatusCode
	}
	if err != nil {
		log.ErrorMsg = err.Error()
	}
	// 异步写入，不阻塞主流程
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
			Type:       "object",
			Properties: t.InputSchema,
			Required:   []string{},
		},
	}
}
