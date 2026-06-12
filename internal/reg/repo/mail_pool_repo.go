package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"web2img/internal/reg/model"
)

// MailPoolFilter 列表过滤。
type MailPoolFilter struct {
	Status   string
	Mode     string
	Keyword  string
	Page     int
	PageSize int
}

// MailPoolRepo 共享邮箱池仓库。
type MailPoolRepo struct{ db *gorm.DB }

// NewMailPoolRepo 构造。
func NewMailPoolRepo(db *gorm.DB) *MailPoolRepo { return &MailPoolRepo{db: db} }

func (r *MailPoolRepo) base(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&model.MailPool{}).Where("deleted_at IS NULL")
}

// List 分页列表。
func (r *MailPoolRepo) List(ctx context.Context, f MailPoolFilter) ([]*model.MailPool, int64, error) {
	q := r.base(ctx)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Mode != "" {
		q = q.Where("mode = ?", f.Mode)
	}
	if f.Keyword != "" {
		q = q.Where("email LIKE ?", "%"+f.Keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, size := normalizePage(f.Page, f.PageSize, 50, 200)
	var rows []*model.MailPool
	if err := q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Stats 各状态计数。
func (r *MailPoolRepo) Stats(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Status string
		Cnt    int64
	}
	var rows []row
	if err := r.base(ctx).Select("status, COUNT(*) as cnt").Group("status").Scan(&rows).Error; err != nil {
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
func (r *MailPoolRepo) GetByID(ctx context.Context, id uint64) (*model.MailPool, error) {
	var row model.MailPool
	err := r.base(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// UpsertMany 批量导入（email 冲突时更新凭据）。
func (r *MailPoolRepo) UpsertMany(ctx context.Context, items []*model.MailPool) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	var affected int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, it := range items {
			var existing model.MailPool
			e := tx.Where("email = ?", it.Email).First(&existing).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				if err := tx.Create(it).Error; err != nil {
					return err
				}
				affected++
				continue
			}
			if e != nil {
				return e
			}
			updates := map[string]any{
				"password_enc":      it.PasswordEnc,
				"client_id":         it.ClientID,
				"refresh_token_enc": it.RefreshTokenEnc,
				"mode":              it.Mode,
				"deleted_at":        nil,
				"updated_at":        time.Now().UTC(),
			}
			if err := tx.Model(&model.MailPool{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
				return err
			}
			affected++
		}
		return nil
	})
	return affected, err
}

// Update 通用字段更新。
func (r *MailPoolRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.MailPool{}).Where("id = ?", id).Updates(fields).Error
}

// SoftDelete 软删单条。
func (r *MailPoolRepo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.MailPool{}).Where("id = ?", id).
		Update("deleted_at", time.Now().UTC()).Error
}

// SoftDeleteByIDs 批量软删。
func (r *MailPoolRepo) SoftDeleteByIDs(ctx context.Context, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&model.MailPool{}).
		Where("id IN ? AND deleted_at IS NULL", ids).Update("deleted_at", time.Now().UTC())
	return res.RowsAffected, res.Error
}

// SoftDeleteByStatus 按状态软删。
func (r *MailPoolRepo) SoftDeleteByStatus(ctx context.Context, status string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.MailPool{}).
		Where("status = ? AND deleted_at IS NULL", status).Update("deleted_at", time.Now().UTC())
	return res.RowsAffected, res.Error
}

// SoftDeleteByFilter 按过滤条件软删（空过滤 = 全部）。
func (r *MailPoolRepo) SoftDeleteByFilter(ctx context.Context, f MailPoolFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&model.MailPool{}).Where("deleted_at IS NULL")
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Mode != "" {
		q = q.Where("mode = ?", f.Mode)
	}
	if f.Keyword != "" {
		q = q.Where("email LIKE ?", "%"+f.Keyword+"%")
	}
	res := q.Update("deleted_at", time.Now().UTC())
	return res.RowsAffected, res.Error
}

// ResetByIDs 批量重置为可用。
func (r *MailPoolRepo) ResetByIDs(ctx context.Context, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Model(&model.MailPool{}).
		Where("id IN ? AND deleted_at IS NULL", ids).Updates(map[string]any{
		"status":             model.MailStatusAvailable,
		"failure_count":      0,
		"last_error":         nil,
		"used_by_provider":   nil,
		"used_by_account_id": nil,
		"used_at":            nil,
	})
	return res.RowsAffected, res.Error
}

// Acquire 领取一条可用邮箱并置为 in_use（事务内 select+update）。
func (r *MailPoolRepo) Acquire(ctx context.Context, provider string) (*model.MailPool, error) {
	var out *model.MailPool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.MailPool
		e := tx.Where("status = ? AND deleted_at IS NULL", model.MailStatusAvailable).
			Order("failure_count ASC, imported_at ASC").First(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		if err := tx.Model(&model.MailPool{}).Where("id = ?", row.ID).Updates(map[string]any{
			"status":           model.MailStatusInUse,
			"used_at":          now,
			"used_by_provider": provider,
		}).Error; err != nil {
			return err
		}
		row.Status = model.MailStatusInUse
		row.UsedAt = &now
		row.UsedByProvider = &provider
		out = &row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Release 归还为可用。
func (r *MailPoolRepo) Release(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.MailPool{}).
		Where("id = ? AND status = ?", id, model.MailStatusInUse).Updates(map[string]any{
		"status":             model.MailStatusAvailable,
		"used_by_provider":   nil,
		"used_by_account_id": nil,
	}).Error
}

// MarkRegistered 标记已注册。
func (r *MailPoolRepo) MarkRegistered(ctx context.Context, id, accountID uint64) error {
	now := time.Now().UTC()
	fields := map[string]any{
		"status":        model.MailStatusRegistered,
		"registered_at": now,
	}
	if accountID > 0 {
		fields["used_by_account_id"] = accountID
	} else {
		fields["used_by_account_id"] = nil
	}
	return r.db.WithContext(ctx).Model(&model.MailPool{}).Where("id = ?", id).Updates(fields).Error
}

// MarkFailed 失败计数 +1；达到上限置 failed，返回是否终态。
func (r *MailPoolRepo) MarkFailed(ctx context.Context, id uint64, errMsg string, maxFail int) (bool, error) {
	if maxFail <= 0 {
		maxFail = 3
	}
	if len(errMsg) > 480 {
		errMsg = errMsg[:480]
	}
	var terminal bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.MailPool
		if e := tx.Where("id = ?", id).First(&row).Error; e != nil {
			return e
		}
		row.FailureCount++
		fields := map[string]any{
			"failure_count": row.FailureCount,
			"last_error":    errMsg,
		}
		if row.FailureCount >= maxFail {
			fields["status"] = model.MailStatusFailed
			terminal = true
		} else {
			fields["status"] = model.MailStatusAvailable
		}
		return tx.Model(&model.MailPool{}).Where("id = ?", id).Updates(fields).Error
	})
	return terminal, err
}

func normalizePage(page, size, def, max int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = def
	}
	if size > max {
		size = max
	}
	return page, size
}
