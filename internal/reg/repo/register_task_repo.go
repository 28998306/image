package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"web2img/internal/reg/model"
)

// RegisterTaskFilter 列表过滤。
type RegisterTaskFilter struct {
	Provider string
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

// PurgeFilter 清理过滤。
type PurgeFilter struct {
	Provider string
	Statuses []string
}

// RegisterTaskRepo 注册任务仓库。
type RegisterTaskRepo struct{ db *gorm.DB }

// NewRegisterTaskRepo 构造。
func NewRegisterTaskRepo(db *gorm.DB) *RegisterTaskRepo { return &RegisterTaskRepo{db: db} }

func (r *RegisterTaskRepo) base(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&model.RegisterTask{}).Where("deleted_at IS NULL")
}

// List 分页列表。
func (r *RegisterTaskRepo) List(ctx context.Context, f RegisterTaskFilter) ([]*model.RegisterTask, int64, error) {
	q := r.base(ctx)
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Keyword != "" {
		q = q.Where("email LIKE ?", "%"+f.Keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := normalizePage(f.Page, f.PageSize, 20, 200)
	var rows []*model.RegisterTask
	if err := q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Stats 各状态计数。
func (r *RegisterTaskRepo) Stats(ctx context.Context, provider string) (map[string]int64, error) {
	type row struct {
		Status string
		Cnt    int64
	}
	q := r.base(ctx)
	if provider != "" {
		q = q.Where("provider = ?", provider)
	}
	var rows []row
	if err := q.Select("status, COUNT(*) as cnt").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]int64{}
	var total int64
	for _, x := range rows {
		out[x.Status] = x.Cnt
		total += x.Cnt
	}
	out["total"] = total
	return out, nil
}

// GetByID 主键查询。
func (r *RegisterTaskRepo) GetByID(ctx context.Context, id uint64) (*model.RegisterTask, error) {
	var row model.RegisterTask
	err := r.base(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// Create 插入。
func (r *RegisterTaskRepo) Create(ctx context.Context, t *model.RegisterTask) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// Update 字段更新。
func (r *RegisterTaskRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.RegisterTask{}).Where("id = ?", id).Updates(fields).Error
}

// SoftDelete 软删。
func (r *RegisterTaskRepo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.RegisterTask{}).Where("id = ?", id).
		Update("deleted_at", time.Now().UTC()).Error
}

// Purge 批量软删已结束任务。
func (r *RegisterTaskRepo) Purge(ctx context.Context, f PurgeFilter) (int64, error) {
	statuses := f.Statuses
	if len(statuses) == 0 {
		statuses = []string{model.RegisterTaskSuccess, model.RegisterTaskFailed, model.RegisterTaskCancelled}
	}
	q := r.db.WithContext(ctx).Model(&model.RegisterTask{}).
		Where("deleted_at IS NULL AND status IN ?", statuses)
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	res := q.Update("deleted_at", time.Now().UTC())
	return res.RowsAffected, res.Error
}

// MarkCancelRequested 置取消请求标志。
func (r *RegisterTaskRepo) MarkCancelRequested(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.RegisterTask{}).Where("id = ?", id).
		Update("cancel_requested", true).Error
}
