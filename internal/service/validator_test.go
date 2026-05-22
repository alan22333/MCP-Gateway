package service

import (
	"encoding/json"
	"testing"

	"github.com/alan22333/mcp-nexus/internal/model"
)

func TestValidateArgsAllValid(t *testing.T) {
	schema := model.JSONMap{
		"name":  map[string]interface{}{"type": "string"},
		"age":   map[string]interface{}{"type": "number"},
		"admin": map[string]interface{}{"type": "boolean"},
	}
	args := json.RawMessage(`{"name":"张三","age":25,"admin":true}`)

	result := validateArgs(schema, args)
	if !result.Valid {
		t.Errorf("所有字段正确应通过: %+v", result.Issues)
	}
}

func TestValidateArgsTypeMismatch(t *testing.T) {
	schema := model.JSONMap{
		"age": map[string]interface{}{"type": "number"},
	}
	args := json.RawMessage(`{"age":"不是数字"}`)

	result := validateArgs(schema, args)
	if result.Valid {
		t.Fatal("age 传了 string 应该失败")
	}
	if len(result.Issues) != 1 {
		t.Fatalf("期望 1 个 issue, 得到 %d", len(result.Issues))
	}
	issue := result.Issues[0]
	if issue.Field != "age" {
		t.Errorf("Field 期望 age, 得到 %s", issue.Field)
	}
	if issue.Expected != "number" {
		t.Errorf("Expected 期望 number, 得到 %s", issue.Expected)
	}
	if issue.Got != "string" {
		t.Errorf("Got 期望 string, 得到 %s", issue.Got)
	}
}

func TestValidateArgsRequiredMissing(t *testing.T) {
	schema := model.JSONMap{
		"order_id": map[string]interface{}{"type": "string", "required": true},
	}
	args := json.RawMessage(`{}`)

	result := validateArgs(schema, args)
	if result.Valid {
		t.Fatal("缺少 required 字段应该失败")
	}
	if !result.Issues[0].Missing {
		t.Errorf("Missing 应为 true")
	}
}

func TestValidateArgsExtraFieldsOK(t *testing.T) {
	// schema 只定义了 name，但 AI 多传了 extra 字段 → 不报错
	schema := model.JSONMap{
		"name": map[string]interface{}{"type": "string"},
	}
	args := json.RawMessage(`{"name":"test","extra":"should be ok"}`)

	result := validateArgs(schema, args)
	if !result.Valid {
		t.Errorf("多余字段不应导致校验失败: %+v", result.Issues)
	}
}

func TestValidateArgsEmptySchema(t *testing.T) {
	// 没有 schema → 总是通过
	result := validateArgs(nil, json.RawMessage(`{"anything":"goes"}`))
	if !result.Valid {
		t.Errorf("空 schema 应通过")
	}
}

func TestValidateArgsInvalidJSON(t *testing.T) {
	schema := model.JSONMap{
		"name": map[string]interface{}{"type": "string"},
	}
	result := validateArgs(schema, json.RawMessage(`not json`))
	if result.Valid {
		t.Fatal("非法 JSON 应失败")
	}
}
