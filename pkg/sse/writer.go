// Package sse 提供 SSE (Server-Sent Events) 流式输出工具
package sse

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Writer 封装向客户端写 SSE 事件的操作
type Writer struct {
	writer  gin.ResponseWriter
	flusher http.Flusher
}

// NewWriter 从 Gin Context 创建 SSE Writer，会自动设置必要的响应头
func NewWriter(c *gin.Context) (*Writer, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用 Nginx 缓冲

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("当前 HTTP 版本不支持 SSE (缺少 Flusher 接口)")
	}

	return &Writer{
		writer:  c.Writer,
		flusher: flusher,
	}, nil
}

// WriteEvent 向客户端写一条 SSE 格式的数据事件
// dataJSON 是已经序列化为 JSON 字符串的数据负载
func (w *Writer) WriteEvent(dataJSON string) error {
	_, err := fmt.Fprintf(w.writer, "data: %s\n\n", dataJSON)
	if err != nil {
		return err
	}
	w.flusher.Flush()
	return nil
}

// WriteComment 写入一条 SSE 注释（仅用于 keep-alive，客户端会忽略）
func (w *Writer) WriteComment(comment string) error {
	_, err := fmt.Fprintf(w.writer, ": %s\n\n", comment)
	if err != nil {
		return err
	}
	w.flusher.Flush()
	return nil
}
