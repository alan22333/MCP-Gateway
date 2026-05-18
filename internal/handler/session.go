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
	GatewayID    uint   // 所属网关 ID
	GatewayName  string // 所属网关名称（日志用）
	Response     chan *mcp.RPCResponse
	Done         chan struct{}
	Limiter      *rate.Limiter // 令牌桶限流器
	LimitEnabled bool

	// 并发控制：用带缓冲 channel 作为信号量
	// 每次 tools/call 先往 channel 发送一个 token，处理完再取出
	// 如果 channel 满了，说明达到并发上限，直接返回 429
	semaphore    chan struct{}
	maxConcurrent int
}

// SessionManager 管理所有活跃的 SSE 会话
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	// 限流配置（所有新 session 共享）
	limiterRPS    float64
	limiterBurst  int
	limitEnabled  bool
	// 并发控制配置
	maxConcurrent int
}

// NewSessionManager 创建 SessionManager 实例
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions:      make(map[string]*Session),
		maxConcurrent: 5, // 默认每 session 最多 5 个并发调用
	}
}

// SetRateLimit 配置限流参数，后续创建的 session 都会带 limiter
// rps: 每秒允许的请求数，burst: 突发容量
func (m *SessionManager) SetRateLimit(rps float64, burst int) {
	m.limitEnabled = true
	m.limiterRPS = rps
	m.limiterBurst = burst
}

// SetConcurrencyLimit 设置每 session 最大并发调用数
// 0 或负数表示不限制
func (m *SessionManager) SetConcurrencyLimit(max int) {
	if max < 1 {
		max = 0
	}
	m.maxConcurrent = max
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
	if m.maxConcurrent > 0 {
		s.maxConcurrent = m.maxConcurrent
		s.semaphore = make(chan struct{}, m.maxConcurrent)
	}
	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()
	return s
}

// TryAcquire 尝试获取并发槽位，失败返回 false（调用方应返回 429）
func (s *Session) TryAcquire() bool {
	if s.semaphore == nil {
		return true // 未启用并发控制，直接放行
	}
	select {
	case s.semaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

// Release 释放并发槽位
func (s *Session) Release() {
	if s.semaphore == nil {
		return
	}
	select {
	case <-s.semaphore:
	default:
	}
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
