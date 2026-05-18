package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"mcp-gateway-go-demo/internal/metrics"
	"mcp-gateway-go-demo/internal/repository"
	"mcp-gateway-go-demo/pkg/mcp"
	"mcp-gateway-go-demo/pkg/sse"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// McpHandler 处理 MCP 相关的 HTTP 请求
type McpHandler struct {
	sessionMgr *SessionManager
	repo       *repository.ApiToolRepo // 用于网关/API Key 查询
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
		repo:       repo,
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
	gatewayID, gatewayName := h.resolveGateway(c)

	session := h.sessionMgr.Create(gatewayID, gatewayName)
	defer h.sessionMgr.Remove(session.ID)

	metrics.ActiveSSESessions.Inc()
	defer metrics.ActiveSSESessions.Dec()

	h.logger.Info("SSE 连接建立",
		zap.String("session_id", session.ID),
		zap.Uint("gateway_id", gatewayID),
		zap.String("gateway_name", gatewayName))

	sessionEvent := map[string]string{"session_id": session.ID}
	payload, _ := json.Marshal(sessionEvent)
	if err := writer.WriteEvent(string(payload)); err != nil {
		return
	}

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

// resolveGateway 从请求参数中解析网关
// 优先级：api_key 查 ApiKey → gateway 查 Gateway → 默认网关
func (h *McpHandler) resolveGateway(c *gin.Context) (uint, string) {
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

	// ── 1. 令牌桶限流 ──
	if session.LimitEnabled && !session.Limiter.Allow() {
		h.logger.Warn("触发限流", zap.String("session_id", sessionID))
		metrics.RecordToolCall("(rate_limited)", "rate_limited", 0)
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

	// ── 2. 并发控制（信号量）──
	// 每个 session 最多同时处理 maxConcurrent 个请求，超过则返回 429
	if !session.TryAcquire() {
		h.logger.Warn("并发限制", zap.String("session_id", sessionID))
		resp := mcp.NewError("concurrency-limit", mcp.ErrCodeInternal,
			fmt.Sprintf("并发请求过多，最多允许 %d 个同时进行的调用", session.maxConcurrent))
		select {
		case session.Response <- resp:
			c.JSON(200, gin.H{"status": "concurrency_limited"})
		default:
			c.JSON(503, gin.H{"error": "响应通道已满"})
		}
		return
	}
	defer session.Release()

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
