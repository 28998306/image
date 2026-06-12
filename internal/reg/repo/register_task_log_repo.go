package repo

import (
	"context"

	"gorm.io/gorm"

	"web2img/internal/reg/model"
)

// RegisterTaskLogFilter 日志过滤。
type RegisterTaskLogFilter struct {
	TaskID   uint64
	Provider string
	Level    string
	Limit    int
}

// RegisterTaskLogRepo 注册任务日志仓库（追加 + 硬删）。
type RegisterTaskLogRepo struct{ db *gorm.DB }

// NewRegisterTaskLogRepo 构造。
func NewRegisterTaskLogRepo(db *gorm.DB) *RegisterTaskLogRepo { return &RegisterTaskLogRepo{db: db} }

// Insert 追加一条日志。
func (r *RegisterTaskLogRepo) Insert(ctx context.Context, l *model.RegisterTaskLog) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// List 倒序查询最近 N 条。
func (r *RegisterTaskLogRepo) List(ctx context.Context, f RegisterTaskLogFilter) ([]*model.RegisterTaskLog, error) {
	q := r.db.WithContext(ctx).Model(&model.RegisterTaskLog{})
	if f.TaskID > 0 {
		q = q.Where("task_id = ?", f.TaskID)
	}
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.Level != "" {
		q = q.Where("level = ?", f.Level)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	var rows []*model.RegisterTaskLog
	if err := q.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// PurgeByTask 删除某任务全部日志。
func (r *RegisterTaskLogRepo) PurgeByTask(ctx context.Context, taskID uint64) error {
	return r.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&model.RegisterTaskLog{}).Error
}

// Purge 按过滤条件硬删日志（空过滤 = 全部）。
func (r *RegisterTaskLogRepo) Purge(ctx context.Context, f RegisterTaskLogFilter) (int64, error) {
	q := r.db.WithContext(ctx)
	if f.TaskID > 0 {
		q = q.Where("task_id = ?", f.TaskID)
	}
	if f.Provider != "" {
		q = q.Where("provider = ?", f.Provider)
	}
	if f.Level != "" {
		q = q.Where("level = ?", f.Level)
	}
	res := q.Delete(&model.RegisterTaskLog{})
	return res.RowsAffected, res.Error
}
