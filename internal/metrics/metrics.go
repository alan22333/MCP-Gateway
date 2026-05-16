// Package metrics 提供 Prometheus 指标定义与注册
// 使用 prometheus client_golang，暴露 /metrics 端点
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ToolCallsTotal 工具调用总数（按 tool_name + status 分组）
	// status: success / validation_error / backend_error / cache_hit / rate_limited
	ToolCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_tool_calls_total",
			Help: "Total number of tool calls, partitioned by tool_name and status.",
		},
		[]string{"tool_name", "status"},
	)

	// ToolCallLatency 工具调用延迟直方图（秒）
	// 只统计实际走后端代理的请求（不含缓存命中和校验失败）
	ToolCallLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_tool_call_duration_seconds",
			Help:    "Latency of backend proxy calls per tool.",
			Buckets: prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		},
		[]string{"tool_name"},
	)

	// CircuitBreakerState 熔断器状态（gauge: 0=closed, 1=open, 2=half-open）
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcp_circuit_breaker_state",
			Help: "Current state of the circuit breaker per group (0=closed, 1=open, 2=half-open).",
		},
		[]string{"group"},
	)

	// ActiveSSESessions SSE 活跃连接数
	ActiveSSESessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mcp_sse_sessions_active",
			Help: "Number of active SSE connections.",
		},
	)
)

// RecordToolCall 记录一次工具调用的指标
// status: "success", "validation_error", "backend_error", "cache_hit", "rate_limited"
// latencySec: 后端代理耗时（秒），非代理调用传 0
func RecordToolCall(toolName, status string, latencySec float64) {
	ToolCallsTotal.WithLabelValues(toolName, status).Inc()
	if latencySec > 0 {
		ToolCallLatency.WithLabelValues(toolName).Observe(latencySec)
	}
}

// SetCircuitBreakerState 更新熔断器状态 gauge
func SetCircuitBreakerState(group, state string) {
	var v float64
	switch state {
	case "closed":
		v = 0
	case "open":
		v = 1
	case "half-open":
		v = 2
	default:
		return
	}
	CircuitBreakerState.WithLabelValues(group).Set(v)
}
