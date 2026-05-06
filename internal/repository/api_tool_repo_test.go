package repository

import (
	"testing"

	"mcp-gateway-go-demo/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB 创建内存 SQLite 数据库用于测试
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("内存数据库创建失败: %v", err)
	}
	return db
}

func TestAutoMigrate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewApiToolRepo(db)
	if err := repo.AutoMigrate(); err != nil {
		t.Fatalf("AutoMigrate 失败: %v", err)
	}
}

func TestCreateAndGetAll(t *testing.T) {
	db := setupTestDB(t)
	repo := NewApiToolRepo(db)
	repo.AutoMigrate()

	tool := &model.ApiTool{
		ToolName:    "get_order",
		Description: "查询订单详情",
		InputSchema: map[string]interface{}{
			"order_id": map[string]interface{}{"type": "string"},
		},
		BackendUrl: "http://api.internal/order",
		HttpMethod: "GET",
	}
	if err := repo.Create(tool); err != nil {
		t.Fatalf("创建工具失败: %v", err)
	}
	if tool.ID == 0 {
		t.Errorf("自增 ID 应该被填充")
	}

	tools, err := repo.GetAll()
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("期望 1 条记录，得到 %d", len(tools))
	}
	if tools[0].ToolName != "get_order" {
		t.Errorf("ToolName 不匹配")
	}
}

func TestGetByToolName(t *testing.T) {
	db := setupTestDB(t)
	repo := NewApiToolRepo(db)
	repo.AutoMigrate()

	t1 := &model.ApiTool{ToolName: "tool_a", Description: "A", InputSchema: nil, BackendUrl: "http://a", HttpMethod: "GET"}
	t2 := &model.ApiTool{ToolName: "tool_b", Description: "B", InputSchema: nil, BackendUrl: "http://b", HttpMethod: "POST"}
	repo.Create(t1)
	repo.Create(t2)

	found, err := repo.GetByToolName("tool_b")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if found.HttpMethod != "POST" {
		t.Errorf("HttpMethod 期望 POST, 得到 %s", found.HttpMethod)
	}

	_, err = repo.GetByToolName("not_exist")
	if err == nil {
		t.Errorf("查询不存在的工具应该返回错误")
	}
}

func TestDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewApiToolRepo(db)
	repo.AutoMigrate()

	tool := &model.ApiTool{ToolName: "to_delete", Description: "x", InputSchema: nil, BackendUrl: "http://x", HttpMethod: "POST"}
	repo.Create(tool)

	if err := repo.Delete(tool.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	tools, _ := repo.GetAll()
	if len(tools) != 0 {
		t.Errorf("删除后应无记录，但还有 %d 条", len(tools))
	}
}

func TestUpdate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewApiToolRepo(db)
	repo.AutoMigrate()

	tool := &model.ApiTool{ToolName: "old_name", Description: "old", InputSchema: nil, BackendUrl: "http://old", HttpMethod: "GET"}
	repo.Create(tool)

	tool.Description = "new_desc"
	if err := repo.Update(tool); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	updated, _ := repo.GetByToolName("old_name")
	if updated.Description != "new_desc" {
		t.Errorf("Description 期望 new_desc, 得到 %s", updated.Description)
	}
}
