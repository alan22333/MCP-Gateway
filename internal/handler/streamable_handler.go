// Streamable HTTP 传输层 handler —— MCP 协议 2025 版统一端点
//
// 与旧版 SSE 传输（GET /mcp/sse + POST /mcp/message）的区别：
//   - 单一 POST /mcp 端点处理所有请求
//   - 支持直接 JSON 响应（application/json）和流式 SSE 响应（text/event-stream）
//   - 通过 Mcp-Session-Id header 管理会话（可选，支持无状态模式）
//   - 支持 JSON-RPC Notification（无 id 字段的请求，返回 202 Accepted）
package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"mcp-gateway-go-demo/internal/metrics"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/pkg/mcp"
	"mcp-gateway-go-demo/pkg/sse"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// StreamableHandler 处理 Streamable HTTP 传输层的 MCP 请求
type StreamableHandler struct {
	sessionMgr *SessionManager
	repo       *repository.ApiToolRepo
	processor  RequestProcessor // 复用 McpService.Process
	logger     *zap.Logger
}

// NewStreamableHandler 创建 Streamable HTTP handler
func NewStreamableHandler(sessionMgr *SessionManager, repo *repository.ApiToolRepo, processor RequestProcessor, logger *zap.Logger) *StreamableHandler {
	return &StreamableHandler{
		sessionMgr: sessionMgr,
		repo:       repo,
		processor:  processor,
		logger:     logger,
	}
}

// Handle 处理 POST /mcp —— Streamable HTTP 统一入口
//
// 请求流程：
//  1. 读取 Mcp-Session-Id header → 查找或创建 session
//  2. 限流检查（令牌桶）
//  3. 并发控制（信号量）
//  4. JSON 解析 → RPCRequest
//  5. Notification 判断（id == nil）→ 202 Accepted
//  6. 网关解析
//  7. 调用 McpService.Process
//  8. 响应路由：Accept: application/json → JSON；Accept: text/event-stream → SSE
func (h *StreamableHandler) Handle(c *gin.Context) {
	// ── 1. Session 解析 ──
	sessionID := c.GetHeader(mcp.HeaderMcpSessionID)
	// 也支持通过 query param 传递 session ID（兼容旧客户端）
	if sessionID == "" {
		sessionID = c.Query("session_id")
	}

	gatewayID, gatewayName := h.resolveGateway(c)
	session := h.sessionMgr.GetOrCreate(sessionID, gatewayID, gatewayName)

	// 如果是新创建的 session（sessionID 为空），在响应头返回 session ID
	if sessionID == "" || sessionID != session.ID {
		c.Header(mcp.HeaderMcpSessionID, session.ID)
	}

	// ── 2. 限流检查 ──
	if session.LimitEnabled && !session.Limiter.Allow() {
		h.logger.Warn("触发限流", zap.String("session_id", session.ID))
		metrics.RecordToolCall("(rate_limited)", "rate_limited", 0)
		resp := mcp.NewError(nil, mcp.ErrCodeInternal,
			"请求过于频繁，请稍后重试 (限流: "+fmt.Sprintf("%.0f req/s)", session.Limiter.Limit()))
		h.writeJSONResponse(c, 429, resp)
		return
	}

	// ── 3. 并发控制 ──
	if !session.TryAcquire() {
		h.logger.Warn("并发限制", zap.String("session_id", session.ID))
		resp := mcp.NewError(nil, mcp.ErrCodeInternal,
			fmt.Sprintf("并发请求过多，最多允许 %d 个同时进行的调用", session.maxConcurrent))
		h.writeJSONResponse(c, 429, resp)
		return
	}
	defer session.Release()

	// ── 4. JSON 解析 ──
	var req mcp.RPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("JSON 解析失败", zap.Error(err))
		c.JSON(400, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}

	h.logger.Info("收到 MCP 请求 (Streamable HTTP)",
		zap.String("method", req.Method),
		zap.Any("id", req.ID),
		zap.String("session_id", session.ID),
		zap.Uint("gateway_id", session.GatewayID),
	)

	// ── 5. Notification 处理 ──
	if req.IsNotification() {
		h.handleNotification(c, &req, session)
		return
	}

	// ── 6. 业务处理 ──
	resp := h.processor.Process(c.Request.Context(), session.GatewayID, &req)

	// ── 7. 响应路由 ──
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "text/event-stream") {
		h.writeSSEResponse(c, resp)
	} else {
		statusCode := 200
		if resp.Error != nil {
			statusCode = 400
		}
		h.writeJSONResponse(c, statusCode, resp)
	}
}

// resolveGateway 从请求参数中解析网关（与 SSE handler 逻辑相同）
// 优先级：api_key query param → gateway query param → 默认网关
func (h *StreamableHandler) resolveGateway(c *gin.Context) (uint, string) {
	// 1. 从 api_key 查
	if apiKey := c.Query("api_key"); apiKey != "" {
		if key, err := h.repo.GetApiKeyByValue(apiKey); err == nil && key != nil {
			gw, gwErr := h.repo.GetGatewayByID(key.GatewayID)
			if gwErr == nil && gw.Enabled {
				return gw.ID, gw.Name
			}
		}
	}

	// 2. 从 gateway 参数查
	if gwName := c.Query("gateway"); gwName != "" {
		if gw, err := h.repo.GetGatewayByName(gwName); err == nil && gw.Enabled {
			return gw.ID, gw.Name
		}
	}

	// 3. 回退：默认网关
	if gw, err := h.repo.GetGatewayByName("Default Gateway"); err == nil {
		return gw.ID, gw.Name
	}

	return 0, "default"
}

// writeJSONResponse 写入 application/json 响应
func (h *StreamableHandler) writeJSONResponse(c *gin.Context, httpStatus int, resp *mcp.RPCResponse) {
	c.Header("Content-Type", "application/json")
	c.JSON(httpStatus, resp)
}

// writeSSEResponse 写入 text/event-stream 流式响应
// 对于 tools/call，先发送 started 事件，再发送 result 事件
func (h *StreamableHandler) writeSSEResponse(c *gin.Context, resp *mcp.RPCResponse) {
	writer, err := sse.NewWriter(c)
	if err != nil {
		h.logger.Warn("SSE Writer 创建失败，回退到 JSON", zap.Error(err))
		h.writeJSONResponse(c, 200, resp)
		return
	}

	payload, _ := json.Marshal(resp)
	if err := writer.WriteEvent(string(payload)); err != nil {
		h.logger.Warn("SSE 写入失败", zap.Error(err))
	}
}

// handleNotification 处理 JSON-RPC Notification（无 id 字段的请求）
// 对于 notifications/initialized：记录日志，返回 202 Accepted
func (h *StreamableHandler) handleNotification(c *gin.Context, req *mcp.RPCRequest, session *Session) {
	switch req.Method {
	case "notifications/initialized":
		h.logger.Info("客户端初始化完成",
			zap.String("session_id", session.ID),
			zap.Uint("gateway_id", session.GatewayID))
		c.JSON(202, gin.H{"status": "accepted"})
	case "notifications/cancelled":
		h.logger.Info("客户端取消请求",
			zap.String("session_id", session.ID))
		c.JSON(202, gin.H{"status": "accepted"})
	default:
		h.logger.Debug("忽略未知通知", zap.String("method", req.Method))
		c.JSON(202, gin.H{"status": "accepted"})
	}
}
