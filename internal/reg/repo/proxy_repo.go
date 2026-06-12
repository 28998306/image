package repo

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"web2img/internal/reg/model"
)

// ProxyRepo 出站代理仓库。
type ProxyRepo struct{ db *gorm.DB }

// NewProxyRepo 构造。
func NewProxyRepo(db *gorm.DB) *ProxyRepo { return &ProxyRepo{db: db} }

// GetByID 主键查询。
func (r *ProxyRepo) GetByID(ctx context.Context, id uint64) (*model.Proxy, error) {
	var row model.Proxy
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// ListEnabled 全部启用代理。
func (r *ProxyRepo) ListEnabled(ctx context.Context) ([]*model.Proxy, error) {
	var rows []*model.Proxy
	err := r.db.WithContext(ctx).Where("deleted_at IS NULL AND status = ?", model.ProxyStatusEnabled).
		Order("id ASC").Find(&rows).Error
	return rows, err
}
