// Package handler 提供 HTTP API 路由与控制器实现
package handler

import (
	"fmt"
	"sync"

	"mcp-gateway-go-demo/pkg/mcp"
	"golang.org/x/time/rate"
)

// Session SSE 会话，每个已连接的大模型客户端对应一个 Session
type Session struct {
	ID           string
	GatewayID    uint             // 所属网关 ID
	GatewayName  string           // 所属网关名称（日志用）
	Response     chan *mcp.RPCResponse
	Done         chan struct{}
	Limiter      *rate.Limiter
	LimitEnabled bool
}

// SessionManager 管理所有活跃的 SSE 会话
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	// 限流配置（所有新 session 共享）
	limiterRPS   float64
	limiterBurst int
	limitEnabled bool
}

// NewSessionManager 创建 SessionManager 实例
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
	}
}

// SetRateLimit 配置限流参数，后续创建的 session 都会带 limiter
// rps: 每秒允许的请求数，burst: 突发容量
func (m *SessionManager) SetRateLimit(rps float64, burst int) {
	m.limitEnabled = true
	m.limiterRPS = rps
	m.limiterBurst = burst
}

// Create 创建一个新会话，绑定到指定网关
func (m *SessionManager) Create(gatewayID uint, gatewayName string) *Session {
	s := &Session{
		ID:          generateSessionID(),
		GatewayID:   gatewayID,
		GatewayName: gatewayName,
		Response:    make(chan *mcp.RPCResponse, 16),
		Done:        make(chan struct{}),
	}
	if m.limitEnabled {
		s.Limiter = rate.NewLimiter(rate.Limit(m.limiterRPS), m.limiterBurst)
		s.LimitEnabled = true
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s
}

// Get 根据 session ID 获取会话
func (m *SessionManager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", id)
	}
	return s, nil
}

// Remove 删除并关闭会话
func (m *SessionManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		close(s.Done)
		delete(m.sessions, id)
	}
}

// SessionInfo 对外暴露的会话信息（不包含 channel）
type SessionInfo struct {
	ID          string `json:"id"`
	GatewayID   uint   `json:"gateway_id"`
	GatewayName string `json:"gateway_name"`
}

// List 返回所有活跃会话的信息
func (m *SessionManager) List() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, SessionInfo{ID: s.ID, GatewayID: s.GatewayID, GatewayName: s.GatewayName})
	}
	return list
}

var sessionCounter int

func generateSessionID() string {
	sessionCounter++
	return fmt.Sprintf("sse-%d", sessionCounter)
}
