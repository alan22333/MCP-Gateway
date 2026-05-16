// Package proxy 封装对后端业务系统的 HTTP 代理请求
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mcp-gateway-go-demo/internal/middleware"

	"github.com/go-resty/resty/v2"
)

// HttpProxy 使用 resty 向后端业务系统发起真实的 HTTP 请求
type HttpProxy struct {
	client *resty.Client
}

// NewHttpProxy 创建 HttpProxy 实例，配置 10 秒超时
func NewHttpProxy() *HttpProxy {
	client := resty.New().
		SetTimeout(10 * time.Second). // 后端请求 10 秒超时
		SetRetryCount(2).             // 失败重试 2 次
		SetRetryWaitTime(500 * time.Millisecond)

	return &HttpProxy{client: client}
}

// ProxyRequest 代理请求参数
type ProxyRequest struct {
	Method string          // HTTP 方法: GET / POST
	URL    string          // 后端地址
	Args   json.RawMessage // AI 传入的参数 JSON
}

// ProxyResponse 代理请求返回
type ProxyResponse struct {
	StatusCode int
	Body       []byte
}

// Forward 发起代理请求到后端业务系统
// ctx 来自上层 HTTP handler，当客户端断开连接时，后端的 resty 请求也会被取消
// 1. URL 中包含 {param} 占位符 → 从 arguments 中提取替换
// 2. GET → 剩余参数展开为 Query；POST → 剩余参数作为 JSON Body
func (p *HttpProxy) Forward(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
	var argsMap map[string]interface{}
	if len(req.Args) > 0 {
		if err := json.Unmarshal(req.Args, &argsMap); err != nil {
			return nil, fmt.Errorf("参数解析失败: %w", err)
		}
	}

	// 替换 URL 中的 {param} 占位符
	url := req.URL
	for k, v := range argsMap {
		placeholder := "{" + k + "}"
		if strings.Contains(url, placeholder) {
			url = strings.ReplaceAll(url, placeholder, fmt.Sprintf("%v", v))
			delete(argsMap, k)
		}
	}

	// SetContext 将上游 context 绑定到 resty 请求，实现全链路超时/取消
	r := p.client.R().SetContext(ctx)

	// 透传 TraceID 到后端，后端日志可以关联到网关的请求
	if traceID := middleware.GetTraceID(ctx); traceID != "" {
		r.SetHeader("X-Request-Id", traceID)
	}

	switch req.Method {
	case "GET":
		for k, v := range argsMap {
			r.SetQueryParam(k, fmt.Sprintf("%v", v))
		}
		resp, err := r.Get(url)
		if err != nil {
			return nil, fmt.Errorf("后端 GET 请求失败: %w", err)
		}
		return &ProxyResponse{StatusCode: resp.StatusCode(), Body: resp.Body()}, nil

	case "POST":
		if len(argsMap) > 0 {
			body, _ := json.Marshal(argsMap)
			r.SetBody(json.RawMessage(body))
		}
		resp, err := r.Post(url)
		if err != nil {
			return nil, fmt.Errorf("后端 POST 请求失败: %w", err)
		}
		return &ProxyResponse{StatusCode: resp.StatusCode(), Body: resp.Body()}, nil

	default:
		return nil, fmt.Errorf("不支持的 HTTP 方法: %s", req.Method)
	}
}
