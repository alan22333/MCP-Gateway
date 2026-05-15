package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestForwardGET(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("期望 GET, 得到 %s", r.Method)
		}
		name := r.URL.Query().Get("name")
		if name != "test" {
			t.Errorf("期望 name=test, 得到 %s", name)
		}
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer backend.Close()

	p := NewHttpProxy()
	resp, err := p.Forward(context.Background(), &ProxyRequest{
		Method: "GET",
		URL:    backend.URL,
		Args:   json.RawMessage(`{"name":"test"}`),
	})
	if err != nil {
		t.Fatalf("GET 请求失败: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("状态码期望 200, 得到 %d", resp.StatusCode)
	}
}

func TestForwardPOST(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("期望 POST, 得到 %s", r.Method)
		}
		w.Write([]byte(`{"created":true}`))
	}))
	defer backend.Close()

	p := NewHttpProxy()
	resp, err := p.Forward(context.Background(), &ProxyRequest{
		Method: "POST",
		URL:    backend.URL,
		Args:   json.RawMessage(`{"data":"value"}`),
	})
	if err != nil {
		t.Fatalf("POST 请求失败: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("状态码期望 200, 得到 %d", resp.StatusCode)
	}
}

func TestForwardTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second)
	}))
	defer slow.Close()

	p := NewHttpProxy()
	_, err := p.Forward(context.Background(), &ProxyRequest{
		Method: "GET",
		URL:    slow.URL,
		Args:   nil,
	})
	if err == nil {
		t.Errorf("超时请求应返回错误")
	}
}

func TestForwardPathParamSubstitution(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/orders/ORD-001" {
			t.Errorf("路径期望 /api/orders/ORD-001, 得到 %s", r.URL.Path)
		}
		if r.URL.Query().Get("status") != "paid" {
			t.Errorf("status query 参数期望 paid, 得到 %s", r.URL.Query().Get("status"))
		}
		w.Write([]byte(`{"order_id":"ORD-001"}`))
	}))
	defer backend.Close()

	p := NewHttpProxy()
	resp, err := p.Forward(context.Background(), &ProxyRequest{
		Method: "GET",
		URL:    backend.URL + "/api/orders/{id}",
		Args:   json.RawMessage(`{"id":"ORD-001","status":"paid"}`),
	})
	if err != nil {
		t.Fatalf("路径替换请求失败: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("状态码期望 200, 得到 %d", resp.StatusCode)
	}
}

func TestForwardInvalidMethod(t *testing.T) {
	p := NewHttpProxy()
	_, err := p.Forward(context.Background(), &ProxyRequest{
		Method: "DELETE",
		URL:    "http://localhost",
		Args:   nil,
	})
	if err == nil {
		t.Errorf("不支持的方法应返回错误")
	}
}

func TestForwardContextCancel(t *testing.T) {
	// 验证：如果上游 context 被取消，代理请求应立即中断
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer backend.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	p := NewHttpProxy()
	_, err := p.Forward(ctx, &ProxyRequest{
		Method: "GET",
		URL:    backend.URL,
	})
	if err == nil {
		t.Errorf("已取消的 context 应导致请求失败")
	}
}
