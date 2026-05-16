// Package proxy 后端代理——含熔断保护
package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

// CBConfig 熔断器配置
type CBConfig struct {
	MaxFailures         int
	Timeout             time.Duration // 熔断持续时间
	HalfOpenMaxRequests int
}

// CircuitBreakerManager 按 backend URL group 管理多个熔断器实例
// 不同 group（/api/orders、/api/customers）互不影响
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*gobreaker.CircuitBreaker
	cfg      CBConfig
}

// NewCircuitBreakerManager 创建熔断器管理器
func NewCircuitBreakerManager(cfg CBConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*gobreaker.CircuitBreaker),
		cfg:      cfg,
	}
}

// getOrCreate 获取或创建指定 group 的熔断器
func (m *CircuitBreakerManager) getOrCreate(group string) *gobreaker.CircuitBreaker {
	m.mu.RLock()
	cb, ok := m.breakers[group]
	m.mu.RUnlock()
	if ok {
		return cb
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// 双重检查
	if cb, ok = m.breakers[group]; ok {
		return cb
	}

	st := gobreaker.Settings{
		Name:        group,
		MaxRequests: uint32(m.cfg.HalfOpenMaxRequests),
		Interval:    m.cfg.Timeout, // 熔断持续多久后进入半开
		Timeout:     m.cfg.Timeout, // 同上

		// ReadyToTrip: 连续失败 N 次 → 打开熔断
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= uint32(m.cfg.MaxFailures) && failureRatio >= 0.6
		},

		// 状态变化日志
		OnStateChange: func(name string, from, to gobreaker.State) {
			// 日志由调用方通过 logger 输出，这里保持静默
			_ = name
			_ = from
			_ = to
		},
	}
	cb = gobreaker.NewCircuitBreaker(st)
	m.breakers[group] = cb
	return cb
}

// Execute 在熔断器保护下执行代理请求
// group: backend URL 路径前缀（如 /api/orders）
// fn: 实际的代理请求逻辑
// 返回值: 代理响应 + error
//   - 如果熔断器打开，直接返回 error 不执行 fn
//   - 如果 fn 返回 error，计为一次失败
func (m *CircuitBreakerManager) Execute(group string, fn func() (*ProxyResponse, error)) (*ProxyResponse, error) {
	cb := m.getOrCreate(group)

	result, err := cb.Execute(func() (interface{}, error) {
		return fn()
	})

	if err != nil {
		// gobreaker 自己的错误（熔断打开）或 fn 的错误
		return nil, err
	}

	return result.(*ProxyResponse), nil
}

// State 返回指定 group 的熔断器状态（供 metrics/调试使用）
func (m *CircuitBreakerManager) State(group string) string {
	cb := m.getOrCreate(group)
	switch cb.State() {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ============ 带熔断的 Forward 方法 ============

// ForwardWithCB 是 HttpProxy.Forward 的熔断包装版本
// 由 service 层调用，自动受熔断保护
// 注意：后端返回 5xx 也被视为失败（resty 只把网络错误当作 error）
func (p *HttpProxy) ForwardWithCB(ctx context.Context, cbManager *CircuitBreakerManager, group string, req *ProxyRequest) (*ProxyResponse, error) {
	return cbManager.Execute(group, func() (*ProxyResponse, error) {
		resp, err := p.Forward(ctx, req)
		if err != nil {
			return nil, err
		}
		// 5xx 视为失败，触发熔断器计数
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("后端返回 %d", resp.StatusCode)
		}
		return resp, nil
	})
}
