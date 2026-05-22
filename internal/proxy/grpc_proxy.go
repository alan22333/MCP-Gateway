// gRPC 动态代理引擎 —— 运行时通过 protobuf reflection 调用任意 gRPC 方法
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// GrpcProxy gRPC 动态代理，通过缓存的 FileDescriptor 在运行时构造和调用 gRPC 方法
type GrpcProxy struct {
	conns map[string]*grpc.ClientConn
	// 缓存从 proto 文件解析的 FileDescriptor (service → file descriptor)
	descriptors map[string]*descriptorpb.FileDescriptorSet
	mu          sync.RWMutex
}

// GrpcRequest gRPC 代理请求
type GrpcRequest struct {
	Addr   string          `json:"addr"`   // gRPC server 地址
	Method string          `json:"method"` // 完整 gRPC 方法路径 "/package.Service/Method"
	Args   json.RawMessage `json:"args"`   // JSON 参数
}

// NewGrpcProxy 创建 gRPC 代理
func NewGrpcProxy() *GrpcProxy {
	return &GrpcProxy{
		conns:       make(map[string]*grpc.ClientConn),
		descriptors: make(map[string]*descriptorpb.FileDescriptorSet),
	}
}

// RegisterProto 注册一个 .proto 文件的 FileDescriptorSet，供后续调用使用
// fds: protoc --descriptor_set_out 生成的 FileDescriptorSet
func (p *GrpcProxy) RegisterProto(serviceName string, fds *descriptorpb.FileDescriptorSet) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.descriptors[serviceName] = fds
}

// GetMethodDescriptor 从已注册的 proto 中查找方法描述符
func (p *GrpcProxy) GetMethodDescriptor(serviceName, methodName string) (protoreflect.MethodDescriptor, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	fds, ok := p.descriptors[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %s not registered (import its .proto first)", serviceName)
	}

	// 从 FileDescriptorSet 构建 registry
	files, err := protodesc.NewFiles(fds)
	if err != nil {
		return nil, fmt.Errorf("build file registry: %w", err)
	}

	// 查找服务（通过完全限定名遍历）
	var sd protoreflect.ServiceDescriptor
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			s := fd.Services().Get(i)
			if s.FullName() == protoreflect.FullName(serviceName) {
				sd = s
				return false
			}
		}
		return true
	})

	if sd == nil {
		return nil, fmt.Errorf("service %s not found in registered descriptors", serviceName)
	}

	md := sd.Methods().ByName(protoreflect.Name(methodName))
	if md == nil {
		return nil, fmt.Errorf("method %s not found in service %s", methodName, serviceName)
	}

	return md, nil
}

// Forward 将 JSON args 转为 protobuf 请求，调用 gRPC 方法，返回 JSON 响应
func (p *GrpcProxy) Forward(ctx context.Context, req *GrpcRequest) (*ProxyResponse, error) {
	conn, err := p.getOrCreateConn(req.Addr)
	if err != nil {
		return nil, fmt.Errorf("grpc connect: %w", err)
	}

	// 解析方法路径 "/package.Service/Method"
	serviceName, methodName := parseMethodPath(req.Method)
	if serviceName == "" {
		return nil, fmt.Errorf("invalid method path: %s", req.Method)
	}

	// 获取方法描述符
	md, err := p.GetMethodDescriptor(serviceName, methodName)
	if err != nil {
		return nil, fmt.Errorf("grpc resolve method: %w", err)
	}

	// JSON args → protobuf request message
	requestMsg := dynamicpb.NewMessage(md.Input())
	if len(req.Args) > 0 {
		if err := protojson.Unmarshal(req.Args, requestMsg); err != nil {
			return nil, fmt.Errorf("grpc unmarshal args: %w", err)
		}
	}

	// 动态调用 gRPC
	responseMsg := dynamicpb.NewMessage(md.Output())
	fullMethod := "/" + serviceName + "/" + methodName
	err = conn.Invoke(ctx, fullMethod, requestMsg, responseMsg)
	if err != nil {
		return &ProxyResponse{
			StatusCode: 2,
			Body:       []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())),
		}, err
	}

	jsonBytes, err := protojson.Marshal(responseMsg)
	if err != nil {
		return nil, fmt.Errorf("grpc marshal response: %w", err)
	}

	return &ProxyResponse{StatusCode: 0, Body: jsonBytes}, nil
}

// Close 关闭所有 gRPC 连接
func (p *GrpcProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = make(map[string]*grpc.ClientConn)
	return nil
}

// getOrCreateConn 连接池
func (p *GrpcProxy) getOrCreateConn(addr string) (*grpc.ClientConn, error) {
	p.mu.RLock()
	conn, ok := p.conns[addr]
	p.mu.RUnlock()
	if ok && conn.GetState() != connectivity.Shutdown {
		return conn, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if conn, ok = p.conns[addr]; ok && conn.GetState() != connectivity.Shutdown {
		return conn, nil
	}

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	p.conns[addr] = conn
	return conn, nil
}

// parseMethodPath "/package.Service/Method" → ("package.Service", "Method")
func parseMethodPath(method string) (string, string) {
	if len(method) == 0 || method[0] != '/' {
		return "", ""
	}
	path := method[1:]
	lastSlash := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash < 0 {
		return "", ""
	}
	return path[:lastSlash], path[lastSlash+1:]
}

// Ensure proto.Message, protoreflect.MethodDescriptor etc are used (avoid import cycle)
var _ proto.Message
var _ protoreflect.MethodDescriptor
