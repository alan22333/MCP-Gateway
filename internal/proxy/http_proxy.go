// Package proxy 封装对后端业务系统的 HTTP 代理请求
package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
// 1. 如果 URL 中包含 {param} 占位符，则从 arguments 中提取对应值替换
// 2. 对于 GET 请求，剩余参数展开为 Query 参数
// 3. 对于 POST 请求，剩余参数作为 JSON Body
func (p *HttpProxy) Forward(req *ProxyRequest) (*ProxyResponse, error) {
	// 先解析 arguments 为 map
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
			delete(argsMap, k) // 已消费的参数不再用作 query/body
		}
	}

	r := p.client.R()

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
