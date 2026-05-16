// Package model 定义数据库模型与领域实体
package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

// JSONMap 自定义类型，用于在 GORM 中存储和读取 JSON 字段
type JSONMap map[string]interface{}

// Scan 实现 sql.Scanner 接口，从数据库读取 JSON
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("类型转换失败: JSONMap 需要 []byte")
	}
	return json.Unmarshal(bytes, j)
}

// Value 实现 driver.Valuer 接口，写入 JSON 到数据库
// nil map 会被序列化为空 JSON 对象 "{}"，避免 NOT NULL 约束冲突
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(j)
}

// ApiTool 工具配置表，记录 AI 可调用的 Tool 与真实 HTTP API 的映射关系
type ApiTool struct {
	gorm.Model
	ToolName    string  `gorm:"uniqueIndex;not null;size:128" json:"tool_name"`       // AI 侧工具名称
	Description string  `gorm:"not null;size:512" json:"description"`                  // 工具功能描述
	InputSchema JSONMap `gorm:"type:json;not null" json:"input_schema"`               // JSON Schema 参数定义
	BackendUrl  string  `gorm:"not null;size:512" json:"backend_url"`                  // 真实被调用的后端地址
	HttpMethod  string  `gorm:"not null;size:10;default:POST" json:"http_method"`      // GET、POST 等
	Enabled     bool    `gorm:"not null;default:true" json:"enabled"`                  // 是否启用（禁用后 AI 看不到该工具）
}

// BeforeCreate GORM 钩子：新建工具时默认启用
func (a *ApiTool) BeforeCreate(tx *gorm.DB) error {
	a.Enabled = true
	return nil
}

// ApiKey API 密钥表，用于网关接入认证
type ApiKey struct {
	gorm.Model
	Key     string `gorm:"uniqueIndex;not null;size:64" json:"key"`  // 密钥值，如 "mcp-gw-sk-xxx"
	Name    string `gorm:"not null;size:128" json:"name"`             // 密钥持有者/用途
	Enabled bool   `gorm:"not null;default:true" json:"enabled"`      // 是否启用
}

// BeforeCreate GORM 钩子：新建时默认启用
func (a *ApiKey) BeforeCreate(tx *gorm.DB) error {
	a.Enabled = true
	return nil
}

// CallLog 调用日志表，记录每次 tools/call 的请求与响应
type CallLog struct {
	gorm.Model
	ToolName     string `gorm:"index;not null;size:128" json:"tool_name"`    // 调用的工具名
	RequestArgs  string `gorm:"type:text" json:"request_args"`                // 请求参数 JSON
	ResponseBody string `gorm:"type:text" json:"response_body"`               // 后端响应内容
	StatusCode   int    `json:"status_code"`                                  // HTTP 状态码
	LatencyMs    int64  `json:"latency_ms"`                                   // 请求耗时（毫秒）
	ErrorMsg     string `gorm:"size:512" json:"error_msg,omitempty"`          // 错误信息
	Caller       string `gorm:"size:32;default:MCP" json:"caller"`            // 调用来源: MCP / WEB
}
