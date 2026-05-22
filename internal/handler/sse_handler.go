package handler

import (
	"context"
	"encoding/json"

	"mcp-gateway-go-demo/internal/metrics"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/pkg/mcp"
	"mcp-gateway-go-demo/pkg/sse"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// McpHandler 处理 MCP 相关的 HTTP 请求（旧版 SSE 传输）
type McpHandler struct {
	sessionMgr *SessionManager
	gwResolver *gatewayResolver // 共享的网关解析器
	processor  RequestProcessor
	logger     *zap.Logger
}

// RequestProcessor 定义处理 JSON-RPC 请求的接口
// gatewayID 标识当前 session 所属网关；ctx 来自 HTTP 请求，用于全链路超时控制
type RequestProcessor interface {
	Process(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse
}

func NewMcpHandler(sessionMgr *SessionManager, repo *repository.ApiToolRepo, processor RequestProcessor, logger *zap.Logger) *McpHandler {
	return &McpHandler{
		sessionMgr: sessionMgr,
		gwResolver: &gatewayResolver{repo: repo},
		processor:  processor,
		logger:     logger,
	}
}

// HandleSSE 处理 GET /mcp/sse —— 建立 SSE 长链接
// 根据 api_key query param 或 gateway query param 确定所属网关
func (h *McpHandler) HandleSSE(c *gin.Context) {
	writer, err := sse.NewWriter(c)
	if err != nil {
		c.JSON(500, gin.H{"error": "SSE 不支持: " + err.Error()})
		return
	}

	// ── 网关解析 ──
	gatewayID, gatewayName := h.gwResolver.resolve(c)

	session := h.sessionMgr.Create(gatewayID, gatewayName)
	defer h.sessionMgr.Remove(session.ID)

	metrics.ActiveSSESessions.Inc()
	defer metrics.ActiveSSESessions.Dec()

	h.logger.Info("SSE 连接建立",
		zap.String("session_id", session.ID),
		zap.Uint("gateway_id", gatewayID),
		zap.String("gateway_name", gatewayName))

	sessionEvent := map[string]string{"session_id": session.ID}
	payload, err := json.Marshal(sessionEvent)
	if err != nil {
		h.logger.Error("序列化 session 事件失败", zap.Error(err))
		return
	}
	if err := writer.WriteEvent(string(payload)); err != nil {
		return
	}

	for {
		select {
		case resp := <-session.Response:
			data, err := json.Marshal(resp)
			if err != nil {
				h.logger.Error("序列化响应失败", zap.Error(err))
				continue
			}
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
// 流程：限流检查 → 并发控制（信号量）→ JSON 解析 → 业务处理 → SSE 响应
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

	// ── 1. 限流 + 并发控制 ──
	if msg := session.CheckGate(); msg != "" {
		h.logger.Warn("流量控制触发", zap.String("session_id", sessionID), zap.String("reason", msg))
		metrics.RecordToolCall("(rate_limited)", "rate_limited", 0)
		resp := mcp.NewError("rate-limited", mcp.ErrCodeInternal, msg)
		select {
		case session.Response <- resp:
			c.JSON(200, gin.H{"status": "rate_limited"})
		default:
			c.JSON(503, gin.H{"error": "响应通道已满"})
		}
		return
	}
	defer session.Release()

	// 把请求体中解析为JSON->JSON-RPC协议
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
		zap.Uint("gateway_id", session.GatewayID),
	)

	resp := h.processor.Process(c.Request.Context(), session.GatewayID, &req)

	select {
	case session.Response <- resp:
		c.JSON(200, gin.H{"status": "ok"})
	default:
		c.JSON(503, gin.H{"error": "响应通道已满，请稍后重试"})
	}
}
