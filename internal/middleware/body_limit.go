package middleware

import (
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit 限制请求体大小，超过 maxBytes 返回 413
// 使用 http.MaxBytesReader 包装 request body，阻止客户端发送过大数据
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 GET/HEAD/OPTIONS，只限制有 body 的请求
		method := c.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			c.Next()
			return
		}

		c.Request.Body = http.MaxBytesReader(
			c.Writer, c.Request.Body, maxBytes,
		)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":     "请求体过大",
				"max_bytes": maxBytes,
				"detail":    fmt.Sprintf("请求体超过 %d 字节 (%d MB)", maxBytes, maxBytes/(1<<20)),
			})
			return
		}

		// 将已读取的 body 放回，供后续 handler 使用
		c.Request.Body = io.NopCloser(
			&readerWithRemain{data: body},
		)
		c.Next()
	}
}

// readerWithRemain 将已读取的字节作为 reader 返回
type readerWithRemain struct {
	data  []byte
	index int
}

func (r *readerWithRemain) Read(p []byte) (int, error) {
	if r.index >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.index:])
	r.index += n
	return n, nil
}

func (r *readerWithRemain) Close() error {
	return nil
}
