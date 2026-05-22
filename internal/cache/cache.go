// Package cache 提供工具调用结果缓存，支持 Redis 和内存两种后端
// 缓存 key 结构: mcp:cache:{group}:{tool_name}:{args_hash}
// group 是 backend URL 路径前缀（如 /api/orders），用于写操作后的批量失效
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// Entry 缓存的单条记录
type Entry struct {
	ToolName string          `json:"tool_name"`
	Args     json.RawMessage `json:"args"`
	Result   json.RawMessage `json:"result"`
	HitAt    time.Time       `json:"hit_at"`
}

// ToolCache 工具调用缓存接口
type ToolCache interface {
	// Get 查询缓存。group 是 backend URL 路径前缀。
	Get(ctx context.Context, group string, toolName string, args json.RawMessage) (*Entry, bool)
	// Set 写入缓存。
	Set(ctx context.Context, group string, toolName string, args json.RawMessage, result json.RawMessage, ttl time.Duration)
	// InvalidateGroup 清除指定 group 下的所有缓存条目
	// 在 POST/PUT/DELETE 写操作成功后调用，防止后续 GET 返回脏数据
	InvalidateGroup(ctx context.Context, group string)
}

// CacheGroup 从 backend URL 提取缓存组标识
// 例: "http://localhost:9090/api/orders/{id}" → "/api/orders"
//
//	"http://localhost:9090/api/customers"  → "/api/customers"
func CacheGroup(backendURL string) string {
	u, err := url.Parse(backendURL)
	if err != nil {
		return backendURL
	}
	p := u.Path
	// 去掉 {param} 占位符所在的最后一段
	last := path.Base(p)
	if strings.Contains(last, "{") {
		p = path.Dir(p)
	}
	// 确保至少有 / 前缀
	if p == "" || p == "." {
		p = "/"
	}
	return p
}

// CacheKey 根据 group + 工具名 + 参数 JSON 生成确定性的缓存 key
// args 会先做 JSON 归一化（反序列化再序列化），消除 key 顺序差异
func CacheKey(group, toolName string, args json.RawMessage) string {
	var normalized interface{}
	if len(args) > 0 {
		json.Unmarshal(args, &normalized)
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		canonical = args // fallback: 使用原始 args
	}
	h := sha256.Sum256(canonical)
	return fmt.Sprintf("mcp:cache:%s:%s:%x", group, toolName, h[:8])
}

// keyPrefix 返回给定 group 的所有 key 的前缀
func keyPrefix(group string) string {
	return fmt.Sprintf("mcp:cache:%s:", group)
}

// ============ 内存实现（无 Redis 时使用）============

type memEntry struct {
	data   json.RawMessage
	expire time.Time
}

// MemCache 基于 map 的内存缓存，支持按 group 批量失效
type MemCache struct {
	mu   sync.RWMutex
	data map[string]*memEntry
}

func NewMemCache() *MemCache {
	mc := &MemCache{data: make(map[string]*memEntry)}
	go func() {
		for {
			time.Sleep(30 * time.Second)
			mc.cleanExpired()
		}
	}()
	return mc
}

func (m *MemCache) cleanExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, v := range m.data {
		if now.After(v.expire) {
			delete(m.data, k)
		}
	}
}

func (m *MemCache) Get(_ context.Context, group, toolName string, args json.RawMessage) (*Entry, bool) {
	key := CacheKey(group, toolName, args)
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.data[key]
	if !ok || time.Now().After(e.expire) {
		return nil, false
	}
	return &Entry{ToolName: toolName, Args: args, Result: e.data, HitAt: time.Now()}, true
}

func (m *MemCache) Set(_ context.Context, group, toolName string, args json.RawMessage, result json.RawMessage, ttl time.Duration) {
	key := CacheKey(group, toolName, args)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = &memEntry{data: result, expire: time.Now().Add(ttl)}
}

func (m *MemCache) InvalidateGroup(_ context.Context, group string) {
	prefix := keyPrefix(group)
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			delete(m.data, k)
		}
	}
}
