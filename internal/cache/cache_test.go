package cache

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCacheKeyDeterministic(t *testing.T) {
	args1 := json.RawMessage(`{"b":2,"a":1}`)
	args2 := json.RawMessage(`{"a":1,"b":2}`)

	key1 := CacheKey("/api/orders", "test", args1)
	key2 := CacheKey("/api/orders", "test", args2)

	if key1 != key2 {
		t.Errorf("相同内容不同顺序应生成相同 key:\n  %s\n  %s", key1, key2)
	}

	key3 := CacheKey("/api/orders", "other", args1)
	if key1 == key3 {
		t.Errorf("不同工具名应生成不同 key")
	}

	key4 := CacheKey("/api/customers", "test", args1)
	if key1 == key4 {
		t.Errorf("不同 group 应生成不同 key")
	}
}

func TestCacheGroup(t *testing.T) {
	tests := []struct{ url, expected string }{
		{"http://localhost:9090/api/orders", "/api/orders"},
		{"http://localhost:9090/api/orders/{id}", "/api/orders"},
		{"http://localhost:9090/api/customers/{id}", "/api/customers"},
		{"http://localhost:9090/api/inventory", "/api/inventory"},
		{"http://localhost:9090/", "/"},
	}
	for _, tc := range tests {
		got := CacheGroup(tc.url)
		if got != tc.expected {
			t.Errorf("CacheGroup(%q) = %q, 期望 %q", tc.url, got, tc.expected)
		}
	}
}

func TestMemCacheSetAndGet(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()
	args := json.RawMessage(`{"customer":"CUST-101"}`)
	result := json.RawMessage(`{"orders":[]}`)

	c.Set(ctx, "/api/orders", "query_orders", args, result, 10*time.Second)

	entry, ok := c.Get(ctx, "/api/orders", "query_orders", args)
	if !ok {
		t.Fatal("刚写入的缓存应命中")
	}
	if string(entry.Result) != string(result) {
		t.Errorf("缓存内容不匹配")
	}
}

func TestMemCacheExpiry(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()
	args := json.RawMessage(`{"x":1}`)
	result := json.RawMessage(`"ok"`)

	c.Set(ctx, "/api/test", "t", args, result, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	_, ok := c.Get(ctx, "/api/test", "t", args)
	if ok {
		t.Errorf("已过期的缓存不应命中")
	}
}

func TestMemCacheMiss(t *testing.T) {
	c := NewMemCache()
	_, ok := c.Get(context.Background(), "/api/x", "nonexistent", json.RawMessage(`{}`))
	if ok {
		t.Errorf("未写入的缓存不应命中")
	}
}

func TestMemCacheInvalidateGroup(t *testing.T) {
	c := NewMemCache()
	ctx := context.Background()

	// 写两个不同 group 的缓存
	c.Set(ctx, "/api/orders", "query_orders", json.RawMessage(`{"c":"CUST-101"}`), json.RawMessage(`"ok"`), 10*time.Second)
	c.Set(ctx, "/api/customers", "query_customers", json.RawMessage(`{}`), json.RawMessage(`"ok"`), 10*time.Second)

	// 清 /api/orders 组
	c.InvalidateGroup(ctx, "/api/orders")

	// /api/orders 组的应消失
	if _, ok := c.Get(ctx, "/api/orders", "query_orders", json.RawMessage(`{"c":"CUST-101"}`)); ok {
		t.Errorf("/api/orders 组已被清空，不应命中")
	}

	// /api/customers 组的应保留
	if _, ok := c.Get(ctx, "/api/customers", "query_customers", json.RawMessage(`{}`)); !ok {
		t.Errorf("/api/customers 组未被清空，应保留")
	}
}
