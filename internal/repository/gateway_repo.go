package repository

import "github.com/alan22333/mcp-nexus/internal/model"

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
