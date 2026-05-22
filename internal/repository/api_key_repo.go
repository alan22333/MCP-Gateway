package repository

import "github.com/alan22333/mcp-nexus/internal/model"

// ====== ApiKey CRUD ======

// CreateApiKey 创建 API 密钥
func (r *ApiToolRepo) CreateApiKey(key *model.ApiKey) error {
	return r.db.Create(key).Error
}

// GetApiKeyByValue 根据密钥值查找已启用的 Key（认证中间件用）
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
