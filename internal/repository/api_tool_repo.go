package repository

import (
	"mcp-gateway-go-demo/internal/model"

	"gorm.io/gorm"
)

// ApiToolRepo ApiTool 的数据访问层
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
	// 迁移：删除旧版单列唯一索引（ToolName），改用复合唯一索引 (gateway_id, tool_name)
	// SQLite 的 AutoMigrate 不会自动删旧索引，需要手动处理
	r.db.Exec("DROP INDEX IF EXISTS idx_api_tools_tool_name")
	return nil
}

// ====== Gateway CRUD ======

func (r *ApiToolRepo) CreateGateway(gw *model.Gateway) error {
	return r.db.Create(gw).Error
}

func (r *ApiToolRepo) GetGatewayByID(id uint) (*model.Gateway, error) {
	var gw model.Gateway
	err := r.db.First(&gw, id).Error
	if err != nil {
		return nil, err
	}
	return &gw, nil
}

func (r *ApiToolRepo) GetGatewayByName(name string) (*model.Gateway, error) {
	var gw model.Gateway
	err := r.db.Where("name = ?", name).First(&gw).Error
	if err != nil {
		return nil, err
	}
	return &gw, nil
}

func (r *ApiToolRepo) ListGateways() ([]model.Gateway, error) {
	var gws []model.Gateway
	err := r.db.Order("id ASC").Find(&gws).Error
	return gws, err
}

func (r *ApiToolRepo) DeleteGateway(id uint) error {
	return r.db.Delete(&model.Gateway{}, id).Error
}

func (r *ApiToolRepo) ToggleGateway(id uint) (*model.Gateway, error) {
	var gw model.Gateway
	if err := r.db.First(&gw, id).Error; err != nil {
		return nil, err
	}
	gw.Enabled = !gw.Enabled
	if err := r.db.Model(&gw).Update("enabled", gw.Enabled).Error; err != nil {
		return nil, err
	}
	return &gw, nil
}

// EnsureDefaultGateway 确保存在默认网关，不存在则创建，返回其 ID
func (r *ApiToolRepo) EnsureDefaultGateway() (uint, error) {
	var gw model.Gateway
	err := r.db.Where("name = ?", "Default Gateway").First(&gw).Error
	if err == nil {
		return gw.ID, nil
	}
	gw = model.Gateway{Name: "Default Gateway", Description: "系统默认网关（自动创建）"}
	if err := r.db.Create(&gw).Error; err != nil {
		return 0, err
	}
	return gw.ID, nil
}

// MigrateToGateway 将 gateway_id=0 的记录批量更新到指定 gatewayID
func (r *ApiToolRepo) MigrateToGateway(gatewayID uint) error {
	r.db.Model(&model.ApiTool{}).Where("gateway_id = 0").Update("gateway_id", gatewayID)
	r.db.Model(&model.ApiKey{}).Where("gateway_id = 0").Update("gateway_id", gatewayID)
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

// GetByToolNameGlobal 全局查找工具名（用于管理后台的 toggle/delete 等通过 ID 的操作，不需要 gateway 过滤）
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

// ToggleEnabled 切换工具的启用/禁用状态，返回更新后的状态
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

// ====== CallLog ======

// CreateCallLog 记录一次工具调用日志
func (r *ApiToolRepo) CreateCallLog(log *model.CallLog) error {
	return r.db.Create(log).Error
}

// GetCallLogs 获取最近的调用日志，limit 条
func (r *ApiToolRepo) GetCallLogs(limit int) ([]model.CallLog, error) {
	var logs []model.CallLog
	err := r.db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

// ====== ApiKey ======

// CreateApiKey 创建 API 密钥
func (r *ApiToolRepo) CreateApiKey(key *model.ApiKey) error {
	return r.db.Create(key).Error
}

// GetApiKeyByValue 根据密钥值查找（认证中间件用，全局搜索）
func (r *ApiToolRepo) GetApiKeyByValue(key string) (*model.ApiKey, error) {
	var ak model.ApiKey
	err := r.db.Where("`key` = ? AND enabled = ?", key, true).First(&ak).Error
	if err != nil {
		return nil, err
	}
	return &ak, nil
}

// GetApiKeysByGateway 返回指定网关的 API 密钥
func (r *ApiToolRepo) GetApiKeysByGateway(gatewayID uint) ([]model.ApiKey, error) {
	var keys []model.ApiKey
	err := r.db.Where("gateway_id = ?", gatewayID).Order("id ASC").Find(&keys).Error
	return keys, err
}

// ListApiKeys 返回所有 API 密钥
func (r *ApiToolRepo) ListApiKeys() ([]model.ApiKey, error) {
	var keys []model.ApiKey
	err := r.db.Order("gateway_id, id").Find(&keys).Error
	return keys, err
}

// DeleteApiKey 删除 API 密钥
func (r *ApiToolRepo) DeleteApiKey(id uint) error {
	return r.db.Delete(&model.ApiKey{}, id).Error
}

// ToggleApiKey 切换密钥启用/禁用
func (r *ApiToolRepo) ToggleApiKey(id uint) (*model.ApiKey, error) {
	var ak model.ApiKey
	if err := r.db.First(&ak, id).Error; err != nil {
		return nil, err
	}
	ak.Enabled = !ak.Enabled
	if err := r.db.Model(&ak).Update("enabled", ak.Enabled).Error; err != nil {
		return nil, err
	}
	return &ak, nil
}

// BatchCreate 批量创建工具（OpenAPI 导入使用）
func (r *ApiToolRepo) BatchCreate(tools []model.ApiTool) (int, error) {
	count := 0
	for _, t := range tools {
		if err := r.db.Create(&t).Error; err != nil {
			// 跳过重复或出错的，继续导入其余
			continue
		}
		count++
	}
	return count, nil
}
