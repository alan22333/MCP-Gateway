package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alan22333/mcp-nexus/internal/repository"
	"github.com/alan22333/mcp-nexus/internal/service"
	"github.com/alan22333/mcp-nexus/pkg/mcp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupStreamableTest(t *testing.T) (*StreamableHandler, *gin.Engine, *repository.ApiToolRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	repo := repository.NewApiToolRepo(db)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	logger := zap.NewNop()
	mgr := NewSessionManager()

	// Create a minimal service (passthrough processor for testing)
	r := gin.New()
	return NewStreamableHandler(mgr, repo, nil, logger), r, repo
}

func TestStreamableHandler_Initialize(t *testing.T) {
	handler, _, _ := setupStreamableTest(t)
	// Override with mock processor
	mockSvc := &mockStreamableProcessor{
		processFunc: func(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse {
			return mcp.NewSuccess(req.ID, &mcp.InitializeResult{
				ProtocolVersion: mcp.ProtocolVersion,
				ServerInfo:      mcp.ServerInfo{Name: "test", Version: "1.0"},
			})
		},
	}
	handler.processor = mockSvc

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/mcp", handler.Handle)

	body := `{"jsonrpc":"2.0","id":"1","method":"initialize"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp mcp.RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error != nil {
		t.Errorf("unexpected error: %s", resp.Error.Message)
	}

	var result mcp.InitializeResult
	resultJSON, _ := json.Marshal(resp.Result)
	json.Unmarshal(resultJSON, &result)
	if result.ProtocolVersion != mcp.ProtocolVersion {
		t.Errorf("expected protocol %s, got %s", mcp.ProtocolVersion, result.ProtocolVersion)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

func TestStreamableHandler_Notification(t *testing.T) {
	handler, _, _ := setupStreamableTest(t)
	handler.processor = &mockStreamableProcessor{}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/mcp", handler.Handle)

	body := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 202 {
		t.Errorf("expected 202 for notification, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStreamableHandler_SessionHeader(t *testing.T) {
	handler, _, _ := setupStreamableTest(t)
	handler.processor = &mockStreamableProcessor{
		processFunc: func(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse {
			return mcp.NewSuccess(req.ID, map[string]string{"ok": "true"})
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/mcp", handler.Handle)

	// 第一次请求 — 无 session ID
	body := `{"jsonrpc":"2.0","id":"1","method":"tools/list"}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	sessionID := w.Header().Get(mcp.HeaderMcpSessionID)
	if sessionID == "" {
		t.Error("expected Mcp-Session-Id header in response")
	}

	// 第二次请求 — 携带 session ID
	req2 := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set(mcp.HeaderMcpSessionID, sessionID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != 200 {
		t.Errorf("expected 200 for second request, got %d", w2.Code)
	}
}

func TestStreamableHandler_SSEAccept(t *testing.T) {
	handler, _, _ := setupStreamableTest(t)
	handler.processor = &mockStreamableProcessor{
		processFunc: func(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse {
			return mcp.NewSuccess(req.ID, map[string]string{"ok": "true"})
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/mcp", handler.Handle)

	body := `{"jsonrpc":"2.0","id":"3","method":"tools/list"}`
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	// SSE 响应应以 "data:" 开头
	if !strings.HasPrefix(w.Body.String(), "data: ") {
		t.Errorf("expected SSE response to start with 'data: ', got: %s", w.Body.String())
	}
}

func TestStreamableHandler_ToolsCall_RealService(t *testing.T) {
	// 使用真实 service（不含 proxy），测试 tools/call 在无后端时的行为
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	repo := repository.NewApiToolRepo(db)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// 创建默认网关和工具
	repo.EnsureDefaultGateway()
	gw, _ := repo.GetGatewayByName("Default Gateway")
	_ = gw // gateway for context

	logger := zap.NewNop()
	mgr := NewSessionManager()

	// 使用真实的 McpService（但 proxy 为 nil —— 只测请求路由）
	svc := service.NewMcpService(repo, nil, nil, nil, nil, 0, logger)

	handler := NewStreamableHandler(mgr, repo, svc, logger)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/mcp", handler.Handle)

	// 调用不存在的工具
	body := `{"jsonrpc":"2.0","id":"1","method":"tools/call","params":{"name":"nonexistent","arguments":{}}}`
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Errorf("expected 400 for nonexistent tool, got %d: %s", w.Code, w.Body.String())
	}

	var resp mcp.RPCResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == nil {
		t.Error("expected error for nonexistent tool")
	}
}

// mockStreamableProcessor 用于测试的模拟处理器
type mockStreamableProcessor struct {
	processFunc func(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse
}

func (m *mockStreamableProcessor) Process(ctx context.Context, gatewayID uint, req *mcp.RPCRequest) *mcp.RPCResponse {
	if m.processFunc != nil {
		return m.processFunc(ctx, gatewayID, req)
	}
	return mcp.NewSuccess(req.ID, map[string]string{"mock": "ok"})
}
