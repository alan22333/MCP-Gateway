package repository

import "github.com/alan22333/mcp-nexus/internal/model"

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
