package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"mcp-gateway-go-demo/internal/metrics"
	"mcp-gateway-go-demo/pkg/mcp"
	"mcp-gateway-go-demo/pkg/sse"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// McpHandler 处理 MCP 相关的 HTTP 请求
type McpHandler struct {
	sessionMgr *SessionManager
	processor  RequestProcessor // 请求处理接口，由 service 层实现
	logger     *zap.Logger
}

// RequestProcessor 定义处理 JSON-RPC 请求的接口
// ctx 来自 HTTP 请求，用于全链路超时控制
type RequestProcessor interface {
	Process(ctx context.Context, req *mcp.RPCRequest) *mcp.RPCResponse
}

// NewMcpHandler 创建 McpHandler 实例
func NewMcpHandler(sessionMgr *SessionManager, processor RequestProcessor, logger *zap.Logger) *McpHandler {
	return &McpHandler{
		sessionMgr: sessionMgr,
		processor:  processor,
		logger:     logger,
	}
}

// HandleSSE 处理 GET /mcp/sse —— 建立 SSE 长链接
// 客户端连接后立即收到一个包含 session_id 的事件，后续通过该 session_id 收发消息
func (h *McpHandler) HandleSSE(c *gin.Context) {
	writer, err := sse.NewWriter(c)
	if err != nil {
		c.JSON(500, gin.H{"error": "SSE 不支持: " + err.Error()})
		return
	}

	session := h.sessionMgr.Create()
	defer h.sessionMgr.Remove(session.ID)

	metrics.ActiveSSESessions.Inc()
	defer metrics.ActiveSSESessions.Dec()

	h.logger.Info("SSE 连接建立", zap.String("session_id", session.ID))

	// 发送 session_id 作为第一个事件，让客户端知道用哪个 ID 来发消息
	sessionEvent := map[string]string{"session_id": session.ID}
	payload, _ := json.Marshal(sessionEvent)
	if err := writer.WriteEvent(string(payload)); err != nil {
		return
	}

	// 进入事件循环，等待服务端推送响应
	for {
		select {
		case resp := <-session.Response:
			data, _ := json.Marshal(resp)
			if err := writer.WriteEvent(string(data)); err != nil {
				h.logger.Warn("SSE 写入失败", zap.Error(err))
				return
			}
		case <-c.Request.Context().Done():
			h.logger.Info("SSE 客户端断开", zap.String("session_id", session.ID))
			return
		case <-session.Done:
			return
		}
	}
}

// HandleMessage 处理 POST /mcp/message —— 接收 JSON-RPC 请求并异步返回结果
// 客户端需要在 query 参数中传入 session_id
func (h *McpHandler) HandleMessage(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(400, gin.H{"error": "缺少 session_id 参数"})
		return
	}

	session, err := h.sessionMgr.Get(sessionID)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// ── 令牌桶限流检查 ──
	if session.LimitEnabled && !session.Limiter.Allow() {
		h.logger.Warn("触发限流", zap.String("session_id", sessionID))
		metrics.RecordToolCall("(rate_limited)", "rate_limited", 0)
		// 返回 MCP 错误格式，走 SSE 推送而非 HTTP 429
		resp := mcp.NewError("rate-limited", mcp.ErrCodeInternal,
			"请求过于频繁，请稍后重试 (限流: "+fmt.Sprintf("%.0f req/s)", session.Limiter.Limit()))
		select {
		case session.Response <- resp:
			c.JSON(200, gin.H{"status": "rate_limited"})
		default:
			c.JSON(503, gin.H{"error": "响应通道已满"})
		}
		return
	}

	var req mcp.RPCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("JSON 解析失败", zap.Error(err))
		c.JSON(400, gin.H{"error": "请求格式错误"})
		return
	}

	h.logger.Info("收到 MCP 请求",
		zap.String("method", req.Method),
		zap.Any("id", req.ID),
		zap.String("session_id", sessionID),
	)

	// 交给 processor（service 层）处理，传入 ctx 实现全链路超时
	resp := h.processor.Process(c.Request.Context(), &req)

	// 将响应推入 SSE 通道
	select {
	case session.Response <- resp:
		c.JSON(200, gin.H{"status": "ok"})
	default:
		// 通道满，客户端消费太慢
		c.JSON(503, gin.H{"error": "响应通道已满，请稍后重试"})
	}
}
