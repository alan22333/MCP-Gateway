// Package handler 提供 HTTP API 路由与控制器实现
package handler

import (
	"fmt"
	"sync"
	"time"

	"mcp-gateway-go-demo/pkg/mcp"

	"github.com/google/uuid"
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
	semaphore     chan struct{}
	maxConcurrent int

	// 生命周期时间戳（Streamable HTTP 使用 TTL 过期机制）
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// Touch 更新最后活跃时间（每次请求时调用）
func (s *Session) Touch() {
	s.LastUsedAt = time.Now()
}

// SessionManager 管理所有活跃会话
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	// 限流配置（所有新 session 共享）
	limiterRPS   float64
	limiterBurst int
	limitEnabled bool
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

// Create 创建一个新会话（自动生成 ID），绑定到指定网关
func (m *SessionManager) Create(gatewayID uint, gatewayName string) *Session {
	return m.CreateWithID(generateSessionID(), gatewayID, gatewayName)
}

// CreateWithID 使用指定 ID 创建会话（Streamable HTTP 通过 Mcp-Session-Id 创建）
func (m *SessionManager) CreateWithID(sessionID string, gatewayID uint, gatewayName string) *Session {
	s := &Session{
		ID:          sessionID,
		GatewayID:   gatewayID,
		GatewayName: gatewayName,
		Response:    make(chan *mcp.RPCResponse, 16),
		Done:        make(chan struct{}),
		CreatedAt:   time.Now(),
		LastUsedAt:  time.Now(),
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

// GetOrCreate 查找已有 session，不存在则创建（Streamable HTTP 模式）
// sessionID 来自 Mcp-Session-Id header；如果为空则创建新 session
func (m *SessionManager) GetOrCreate(sessionID string, gatewayID uint, gatewayName string) *Session {
	if sessionID != "" {
		m.mu.RLock()
		s, ok := m.sessions[sessionID]
		m.mu.RUnlock()
		if ok {
			s.Touch()
			return s
		}
	}
	// 不存在或无 sessionID → 创建
	id := sessionID
	if id == "" {
		id = uuid.New().String()
	}
	return m.CreateWithID(id, gatewayID, gatewayName)
}

// CheckGate 检查限流和并发控制，通过则获取并发槽位并返回 nil
// 失败时返回用户可读的错误消息，调用方应终止处理并返回该消息
func (s *Session) CheckGate() string {
	if s.LimitEnabled && !s.Limiter.Allow() {
		return fmt.Sprintf("请求过于频繁，请稍后重试 (限流: %.0f req/s)", s.Limiter.Limit())
	}
	if !s.TryAcquire() {
		return fmt.Sprintf("并发请求过多，最多允许 %d 个同时进行的调用", s.maxConcurrent)
	}
	return ""
}

// TryAcquire 尝试获取并发槽位，失败返回 false
func (s *Session) TryAcquire() bool {
	if s.semaphore == nil {
		return true
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

// CleanupExpired 后台 goroutine：每 60 秒扫描一次，删除超过 ttl 未活跃的 session
// 通过 done channel 控制退出
func (m *SessionManager) CleanupExpired(ttl time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			m.mu.Lock()
			for id, s := range m.sessions {
				if now.Sub(s.LastUsedAt) > ttl {
					close(s.Done)
					delete(m.sessions, id)
				}
			}
			m.mu.Unlock()
		case <-done:
			return
		}
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
