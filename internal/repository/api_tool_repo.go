package repository

import (
	"mcp-gateway-go-demo/internal/model"

	"gorm.io/gorm"
)

// ApiToolRepo 数据访问层，管理 Gateway / ApiTool / ApiKey / CallLog 四个实体。
// 方法按实体拆分到不同文件：gateway_repo.go / api_key_repo.go / call_log_repo.go
type ApiToolRepo struct {
	db *gorm.DB
}

func NewApiToolRepo(db *gorm.DB) *ApiToolRepo {
	return &ApiToolRepo{db: db}
}

func (r *ApiToolRepo) AutoMigrate() error {
	if err := r.db.AutoMigrate(&model.Gateway{}, &model.ApiTool{}, &model.CallLog{}, &model.ApiKey{}); err != nil {
		return err
	}
	// SQLite 的 AutoMigrate 不会自动删旧索引，需要手动处理
	r.db.Exec("DROP INDEX IF EXISTS idx_api_tools_tool_name")
	return nil
}

// ====== ApiTool CRUD ======

func (r *ApiToolRepo) Create(tool *model.ApiTool) error {
	return r.db.Create(tool).Error
}

// GetAllTools 返回所有工具（管理后台用，不限 gateway）
func (r *ApiToolRepo) GetAllTools() ([]model.ApiTool, error) {
	var tools []model.ApiTool
	err := r.db.Order("gateway_id, id").Find(&tools).Error
	return tools, err
}

// GetEnabled 返回所有已启用的工具（保持向后兼容）
func (r *ApiToolRepo) GetEnabled() ([]model.ApiTool, error) {
	var tools []model.ApiTool
	err := r.db.Where("enabled = ?", true).Find(&tools).Error
	return tools, err
}

// GetToolsByGateway 返回指定网关下已启用的工具（tools/list 使用）
func (r *ApiToolRepo) GetToolsByGateway(gatewayID uint) ([]model.ApiTool, error) {
	var tools []model.ApiTool
	err := r.db.Where("gateway_id = ? AND enabled = ?", gatewayID, true).Find(&tools).Error
	return tools, err
}

func (r *ApiToolRepo) GetByID(id uint) (*model.ApiTool, error) {
	var tool model.ApiTool
	err := r.db.First(&tool, id).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// GetByToolName 在指定网关下按名称查找工具
func (r *ApiToolRepo) GetByToolName(gatewayID uint, name string) (*model.ApiTool, error) {
	var tool model.ApiTool
	err := r.db.Where("gateway_id = ? AND tool_name = ?", gatewayID, name).First(&tool).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

// GetByToolNameGlobal 全局查找工具名（管理后台 toggle/delete 用，不需要 gateway 过滤）
func (r *ApiToolRepo) GetByToolNameGlobal(name string) (*model.ApiTool, error) {
	var tool model.ApiTool
	err := r.db.Where("tool_name = ?", name).First(&tool).Error
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

func (r *ApiToolRepo) Update(tool *model.ApiTool) error {
	return r.db.Save(tool).Error
}

// ToggleEnabled 切换工具的启用/禁用状态，返回更新后的工具
func (r *ApiToolRepo) ToggleEnabled(id uint) (*model.ApiTool, error) {
	tool, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}
	tool.Enabled = !tool.Enabled
	if err := r.db.Model(tool).Update("enabled", tool.Enabled).Error; err != nil {
		return nil, err
	}
	return tool, nil
}

func (r *ApiToolRepo) Delete(id uint) error {
	return r.db.Delete(&model.ApiTool{}, id).Error
}

// BatchCreate 批量创建工具（OpenAPI 导入使用），跳过失败项继续导入
func (r *ApiToolRepo) BatchCreate(tools []model.ApiTool) (int, error) {
	count := 0
	for _, t := range tools {
		if err := r.db.Create(&t).Error; err != nil {
			continue
		}
		count++
	}
	return count, nil
}
