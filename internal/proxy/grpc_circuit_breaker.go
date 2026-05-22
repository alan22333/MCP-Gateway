package proxy

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// circuitBreakerProxy 为任意代理类型提供熔断包装的接口
type circuitBreakerProxy interface {
	Forward(ctx context.Context, req *GrpcRequest) (*ProxyResponse, error)
}

// ForwardWithCB 带熔断保护的 gRPC 代理转发
// 失败判定：gRPC status code 为 Internal, Unavailable, DeadlineExceeded 时计为失败
func (p *GrpcProxy) ForwardWithCB(ctx context.Context, cbManager *CircuitBreakerManager, group string, req *GrpcRequest) (*ProxyResponse, error) {
	result, err := cbManager.Execute(group, func() (*ProxyResponse, error) {
		resp, fwdErr := p.Forward(ctx, req)
		if fwdErr != nil {
			return nil, fwdErr
		}
		// gRPC 非 OK 状态码 → 计为失败
		st, ok := status.FromError(fwdErr)
		if ok {
			code := st.Code()
			if code == codes.Internal || code == codes.Unavailable || code == codes.DeadlineExceeded {
				return nil, fmt.Errorf("grpc error: %s (code=%d)", st.Message(), code)
			}
		}
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
