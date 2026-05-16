package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gobreaker "github.com/sony/gobreaker"
)

func TestCircuitBreakerClosedToOpen(t *testing.T) {
	cb := NewCircuitBreakerManager(CBConfig{
		MaxFailures:         3,
		Timeout:             100 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	p := NewHttpProxy()
	group := "/api/test"

	for i := 0; i < 3; i++ {
		_, err := p.ForwardWithCB(nil, cb, group, &ProxyRequest{Method: "GET", URL: backend.URL})
		if err == nil {
			t.Fatalf("第 %d 次请求应返回错误", i+1)
		}
	}

	if cb.State(group) != "open" {
		t.Errorf("连续失败 3 次后熔断器应为 open, 实际: %s", cb.State(group))
	}
}

func TestCircuitBreakerHalfOpenRecovery(t *testing.T) {
	cb := NewCircuitBreakerManager(CBConfig{
		MaxFailures:         2,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 1,
	})

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer bad.Close()
	group := "/api/recovery"

	p := NewHttpProxy()
	for i := 0; i < 3; i++ {
		p.ForwardWithCB(nil, cb, group, &ProxyRequest{Method: "GET", URL: bad.URL})
	}
	if cb.State(group) != "open" {
		t.Fatalf("熔断器应打开，实际: %s", cb.State(group))
	}

	time.Sleep(100 * time.Millisecond)

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`"ok"`))
	}))
	defer good.Close()

	for i := 0; i < 5; i++ {
		_, err := p.ForwardWithCB(nil, cb, group, &ProxyRequest{Method: "GET", URL: good.URL})
		if err != nil {
			t.Logf("第 %d 次: %v (state=%s)", i+1, err, cb.State(group))
		}
	}

	if cb.State(group) != "closed" {
		t.Errorf("恢复后熔断器应为 closed, 实际: %s", cb.State(group))
	}
}

func TestCircuitBreakerGroupIsolation(t *testing.T) {
	cb := NewCircuitBreakerManager(CBConfig{
		MaxFailures:         1,
		Timeout:             10 * time.Second,
		HalfOpenMaxRequests: 1,
	})

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer bad.Close()

	p := NewHttpProxy()
	p.ForwardWithCB(nil, cb, "/api/orders", &ProxyRequest{Method: "GET", URL: bad.URL})

	if cb.State("/api/customers") != "closed" {
		t.Errorf("不同 group 应独立熔断")
	}
	if cb.State("/api/orders") != "open" {
		t.Errorf("/api/orders 组应 open")
	}
}

func TestCircuitBreakerErrOpenState(t *testing.T) {
	cb := NewCircuitBreakerManager(CBConfig{MaxFailures: 1, Timeout: 10 * time.Second, HalfOpenMaxRequests: 1})

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer bad.Close()

	p := NewHttpProxy()
	p.ForwardWithCB(nil, cb, "/api/x", &ProxyRequest{Method: "GET", URL: bad.URL})

	_, err := p.ForwardWithCB(nil, cb, "/api/x", &ProxyRequest{Method: "GET", URL: bad.URL})
	if err == nil {
		t.Fatal("熔断打开后应返回错误")
	}
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Errorf("错误应为 ErrOpenState, 实际: %v", err)
	}
}
