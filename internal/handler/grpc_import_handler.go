package handler

import (
	"mcp-gateway-go-demo/internal/model"
	"mcp-gateway-go-demo/internal/proxy"
	"mcp-gateway-go-demo/pkg/protobuf"

	"github.com/gin-gonic/gin"
)

// GrpcImportHandler gRPC proto 文件导入 handler
type GrpcImportHandler struct {
	repo      grpcImportRepo
	grpcProxy *proxy.GrpcProxy
}

// grpcImportRepo GrpcImportHandler 需要的 repo 方法子集
type grpcImportRepo interface {
	GetGatewayByID(id uint) (*model.Gateway, error)
	BatchCreate(tools []model.ApiTool) (int, error)
}

// NewGrpcImportHandler 创建 gRPC 导入 handler
func NewGrpcImportHandler(repo grpcImportRepo, grpcProxy *proxy.GrpcProxy) *GrpcImportHandler {
	return &GrpcImportHandler{repo: repo, grpcProxy: grpcProxy}
}

// RegisterRoutes 注册路由
func (h *GrpcImportHandler) RegisterRoutes(r gin.IRouter) {
	r.POST("/tools/import-grpc", h.Import)
}

// Import 处理 POST /api/tools/import-grpc
// 接受 JSON: {"proto_content": "...", "addr": "localhost:50051", "gateway_id": 1}
func (h *GrpcImportHandler) Import(c *gin.Context) {
	var input struct {
		ProtoContent string `json:"proto_content"` // .proto 文件文本内容
		Addr         string `json:"addr"`          // gRPC server 地址
		GatewayID    uint   `json:"gateway_id"`     // 所属网关
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "请求格式错误: " + err.Error()})
		return
	}
	if input.ProtoContent == "" {
		c.JSON(400, gin.H{"error": "缺少 proto_content 字段"})
		return
	}
	if input.Addr == "" {
		c.JSON(400, gin.H{"error": "缺少 addr 字段 (gRPC server 地址)"})
		return
	}

	// 解析 .proto 文件
	result, err := protobuf.ParseProto(input.ProtoContent, "api.proto")
	if err != nil {
		c.JSON(400, gin.H{"error": "proto 解析失败: " + err.Error()})
		return
	}

	// 注册 FileDescriptorSet 到 GrpcProxy（供后续调用）
	if result.FDS != nil && len(result.Services) > 0 {
		for _, svc := range result.Services {
			h.grpcProxy.RegisterProto(svc, result.FDS)
		}
	}

	// 预览模式
	if c.Query("preview") == "true" {
		c.JSON(200, gin.H{
			"services": result.Services,
			"methods":  result.Methods,
			"addr":     input.Addr,
		})
		return
	}

	// 转换为 ApiTool 并批量创建
	tools := make([]model.ApiTool, 0, len(result.Methods))
	for _, m := range result.Methods {
		tools = append(tools, model.ApiTool{
			GatewayID:   input.GatewayID,
			ToolName:    m.ToolName,
			Description: m.Description,
			InputSchema: model.JSONMap(m.InputSchema),
			BackendUrl:  input.Addr,
			HttpMethod:  m.MethodPath,
			Protocol:    "grpc",
		})
	}

	count, err := h.repo.BatchCreate(tools)
	if err != nil {
		c.JSON(500, gin.H{"error": "创建工具失败: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"message": "gRPC 工具导入完成",
		"created": count,
		"total":   len(tools),
	})
}
