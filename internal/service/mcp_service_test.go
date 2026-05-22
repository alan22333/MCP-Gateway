package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alan22333/mcp-nexus/internal/cache"
	"github.com/alan22333/mcp-nexus/internal/model"
	"github.com/alan22333/mcp-nexus/internal/proxy"
	"github.com/alan22333/mcp-nexus/internal/repository"
	"github.com/alan22333/mcp-nexus/pkg/mcp"

	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupService(t *testing.T) *McpService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("内存数据库创建失败: %v", err)
	}
	repo := repository.NewApiToolRepo(db)
	repo.AutoMigrate()

	repo.Create(&model.ApiTool{
		ToolName:    "echo",
		Description: "回显测试工具",
		InputSchema: map[string]interface{}{
			"message": map[string]interface{}{"type": "string"},
		},
		BackendUrl: "",
		HttpMethod: "POST",
	})

	cbMgr := proxy.NewCircuitBreakerManager(proxy.CBConfig{MaxFailures: 5, Timeout: 30 * time.Second, HalfOpenMaxRequests: 1})
	return NewMcpService(repo, proxy.NewHttpProxy(), nil, cbMgr, cache.NewMemCache(), 60*time.Second, zap.NewNop())
}

func TestHandleInitialize(t *testing.T) {
	svc := setupService(t)
	req := &mcp.RPCRequest{JSONRPC: "2.0", ID: "1", Method: "initialize"}
	resp := svc.Process(context.Background(), 0, req)
	if resp.Error != nil {
		t.Fatalf("initialize 不应返回错误: %+v", resp.Error)
	}
}

func TestHandleToolsList(t *testing.T) {
	svc := setupService(t)
	req := &mcp.RPCRequest{JSONRPC: "2.0", ID: "2", Method: "tools/list"}
	resp := svc.Process(context.Background(), 0, req)
	if resp.Error != nil {
		t.Fatalf("tools/list 不应返回错误: %+v", resp.Error)
	}

	// 验证返回类型
	resultBytes, _ := json.Marshal(resp.Result)
	var result mcp.ToolsListResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("result 解析失败: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Errorf("期望 1 个工具, 得到 %d", len(result.Tools))
	}
}

func TestHandleToolsCallNotFound(t *testing.T) {
	svc := setupService(t)
	params, _ := json.Marshal(mcp.CallToolParams{Name: "nonexistent", Arguments: nil})
	req := &mcp.RPCRequest{JSONRPC: "2.0", ID: "3", Method: "tools/call", Params: params}
	resp := svc.Process(context.Background(), 0, req)
	if resp.Error == nil {
		t.Fatalf("调用不存在的工具应返回错误")
	}
}

func TestHandleToolsCallSuccess(t *testing.T) {
	// 启动一个模拟后端服务
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result": "ok", "echo": "hello"}`))
	}))
	defer backend.Close()

	// 创建带模拟后端地址的 service
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	repo := repository.NewApiToolRepo(db)
	repo.AutoMigrate()
	repo.Create(&model.ApiTool{
		ToolName:    "echo",
		Description: "回显",
		InputSchema: nil,
		BackendUrl:  backend.URL,
		HttpMethod:  "POST",
	})

	cbMgr := proxy.NewCircuitBreakerManager(proxy.CBConfig{MaxFailures: 5, Timeout: 30 * time.Second, HalfOpenMaxRequests: 1})
	svc := NewMcpService(repo, proxy.NewHttpProxy(), nil, cbMgr, cache.NewMemCache(), 60*time.Second, zap.NewNop())

	params, _ := json.Marshal(mcp.CallToolParams{
		Name:      "echo",
		Arguments: json.RawMessage(`{"message":"hello"}`),
	})
	req := &mcp.RPCRequest{JSONRPC: "2.0", ID: "4", Method: "tools/call", Params: params}
	resp := svc.Process(context.Background(), 0, req)
	if resp.Error != nil {
		t.Fatalf("tools/call 不应返回错误: %+v", resp.Error)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result mcp.CallToolResult
	json.Unmarshal(resultBytes, &result)
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Errorf("返回格式不正确")
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	svc := setupService(t)
	req := &mcp.RPCRequest{JSONRPC: "2.0", ID: "5", Method: "unknown/thing"}
	resp := svc.Process(context.Background(), 0, req)
	if resp.Error == nil {
		t.Fatalf("未知方法应返回错误")
	}
	if resp.Error.Code != mcp.ErrCodeMethod {
		t.Errorf("错误码期望 %d, 得到 %d", mcp.ErrCodeMethod, resp.Error.Code)
	}
}
