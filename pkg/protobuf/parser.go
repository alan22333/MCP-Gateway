// Package protobuf 提供 .proto 文件解析，将 gRPC service 方法转换为 MCP Tool 定义
package protobuf

import (
	"fmt"
	"sort"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ParsedMethod .proto 文件中一个 RPC 方法的解析结果
type ParsedMethod struct {
	ToolName    string                 `json:"tool_name"`    // "OrderService.GetOrder"
	Description string                 `json:"description"`  // 方法描述
	InputSchema map[string]interface{} `json:"input_schema"` // JSON Schema 参数定义
	ServiceName string                 `json:"service_name"` // 完整限定服务名
	MethodName  string                 `json:"method_name"`  // 方法名
	MethodPath  string                 `json:"method_path"`  // "/package.Service/Method"
}

// ParseResult .proto 文件解析结果
type ParseResult struct {
	Methods  []ParsedMethod                  `json:"methods"`
	Services []string                        `json:"services"`
	FDS      *descriptorpb.FileDescriptorSet `json:"-"` // 编译后的描述符集，供 GrpcProxy 使用
}

// ParseProto 解析 .proto 源文件文本，返回可导入的 gRPC 方法列表
func ParseProto(protoContent string, filename string) (*ParseResult, error) {
	parser := protoparse.Parser{
		Accessor: protoparse.FileContentsFromMap(map[string]string{
			filename: protoContent,
		}),
	}

	fds, err := parser.ParseFiles(filename)
	if err != nil {
		return nil, fmt.Errorf("parse proto: %w", err)
	}
	if len(fds) == 0 {
		return nil, fmt.Errorf("no files parsed from %s", filename)
	}

	fd := fds[0]
	result := &ParseResult{
		Services: make([]string, 0),
		Methods:  make([]ParsedMethod, 0),
	}

	// 构建 FileDescriptorSet
	fdp := fd.AsFileDescriptorProto()
	result.FDS = &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{fdp},
	}

	// 遍历 service → method
	for _, sd := range fd.GetServices() {
		svcName := sd.GetFullyQualifiedName()
		result.Services = append(result.Services, svcName)

		for _, md := range sd.GetMethods() {
			methodName := md.GetName()
			methodPath := "/" + svcName + "/" + methodName

			parsed := ParsedMethod{
				ToolName:    methodName,
				Description: fmt.Sprintf("gRPC method %s.%s", svcName, methodName),
				ServiceName: svcName,
				MethodName:  methodName,
				MethodPath:  methodPath,
				InputSchema: buildInputSchema(md),
			}
			result.Methods = append(result.Methods, parsed)
		}
	}

	sort.Slice(result.Methods, func(i, j int) bool {
		return result.Methods[i].MethodPath < result.Methods[j].MethodPath
	})

	return result, nil
}

// buildInputSchema 从方法描述符构建 MCP JSON Schema 参数定义
func buildInputSchema(md *desc.MethodDescriptor) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
	}

	inputType := md.GetInputType()
	if inputType == nil {
		return schema
	}

	props := schema["properties"].(map[string]interface{})
	for _, f := range inputType.GetFields() {
		props[f.GetName()] = fieldToJSONSchema(f)
	}

	return schema
}

// fieldToJSONSchema 将 protobuf 字段描述符转为 JSON Schema 类型定义
func fieldToJSONSchema(f *desc.FieldDescriptor) map[string]interface{} {
	itemSchema := map[string]interface{}{}

	switch f.GetType().Number() {
	case descriptorpb.FieldDescriptorProto_TYPE_STRING.Number():
		itemSchema["type"] = "string"
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Number():
		itemSchema["type"] = "number"
	case descriptorpb.FieldDescriptorProto_TYPE_INT32.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_INT64.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_UINT32.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_UINT64.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_SINT32.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_SINT64.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32.Number(),
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64.Number():
		itemSchema["type"] = "integer"
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL.Number():
		itemSchema["type"] = "boolean"
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES.Number():
		itemSchema["type"] = "string"
		itemSchema["format"] = "byte"
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Number():
		itemSchema["type"] = "object"
		itemSchema["properties"] = make(map[string]interface{})
		if msgType := f.GetMessageType(); msgType != nil {
			for _, nf := range msgType.GetFields() {
				itemSchema["properties"].(map[string]interface{})[nf.GetName()] = fieldToJSONSchema(nf)
			}
		}
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM.Number():
		itemSchema["type"] = "string"
		if enumType := f.GetEnumType(); enumType != nil {
			vals := make([]string, 0)
			for _, ev := range enumType.GetValues() {
				vals = append(vals, ev.GetName())
			}
			itemSchema["enum"] = vals
		}
	default:
		itemSchema["type"] = "string"
	}

	if f.IsRepeated() {
		return map[string]interface{}{
			"type":  "array",
			"items": itemSchema,
		}
	}

	return itemSchema
}
