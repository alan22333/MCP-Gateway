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
	return r.db.AutoMigrate(&model.ApiTool{}, &model.CallLog{}, &model.ApiKey{})
}

func (r *ApiToolRepo) Create(tool *model.ApiTool) error {
	return r.db.Create(tool).Error
}

func (r *ApiToolRepo) GetAll() ([]model.ApiTool, error) {
	var tools []model.ApiTool
	err := r.db.Find(&tools).Error
	return tools, err
}

// GetEnabled 只返回已启用的工具（tools/list 使用）
func (r *ApiToolRepo) GetEnabled() ([]model.ApiTool, error) {
	var tools []model.ApiTool
	err := r.db.Where("enabled = ?", true).Find(&tools).Error
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

func (r *ApiToolRepo) GetByToolName(name string) (*model.ApiTool, error) {
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

// GetApiKeyByValue 根据密钥值查找（认证中间件用）
func (r *ApiToolRepo) GetApiKeyByValue(key string) (*model.ApiKey, error) {
	var ak model.ApiKey
	err := r.db.Where("`key` = ? AND enabled = ?", key, true).First(&ak).Error
	if err != nil {
		return nil, err
	}
	return &ak, nil
}

// ListApiKeys 返回所有 API 密钥
func (r *ApiToolRepo) ListApiKeys() ([]model.ApiKey, error) {
	var keys []model.ApiKey
	err := r.db.Find(&keys).Error
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
