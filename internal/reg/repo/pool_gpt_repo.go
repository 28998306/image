package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"web2img/internal/reg/model"
)

// PoolGptFilter 列表过滤。
type PoolGptFilter struct {
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

// PoolGptRepo GPT 号池仓库。
type PoolGptRepo struct{ db *gorm.DB }

// NewPoolGptRepo 构造。
func NewPoolGptRepo(db *gorm.DB) *PoolGptRepo { return &PoolGptRepo{db: db} }

func (r *PoolGptRepo) base(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&model.PoolGpt{}).Where("deleted_at IS NULL")
}

// List 分页列表。
func (r *PoolGptRepo) List(ctx context.Context, f PoolGptFilter) ([]*model.PoolGpt, int64, error) {
	q := r.base(ctx)
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
	page, size := normalizePage(f.Page, f.PageSize, 50, 10000)
	var rows []*model.PoolGpt
	if err := q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// Stats 各状态计数。
func (r *PoolGptRepo) Stats(ctx context.Context) (map[string]int64, error) {
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

// GetByEmail 按邮箱查询（用于导入去重 upsert）。未命中返回 ErrNotFound。
func (r *PoolGptRepo) GetByEmail(ctx context.Context, email string) (*model.PoolGpt, error) {
	var row model.PoolGpt
	err := r.base(ctx).Where("email = ?", email).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// SoftDeleteByStatus 按状态批量硬删（用于「删除失效」）。
func (r *PoolGptRepo) SoftDeleteByStatus(ctx context.Context, status string) (int64, error) {
	if status == "" {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("status = ?", status).Delete(&model.PoolGpt{})
	return res.RowsAffected, res.Error
}

// ListRefreshable 列出可刷新（有 refresh_token）的账号，按到期时间升序。
// withinExpirySeconds > 0 时只返回在该秒数内到期或已过期的账号；<=0 返回全部可刷新账号。
func (r *PoolGptRepo) ListRefreshable(ctx context.Context, withinExpirySeconds int64, max int) ([]*model.PoolGpt, error) {
	q := r.base(ctx).Where("refresh_token_enc IS NOT NULL").Where("status <> ?", model.GPTStatusDisabled)
	if withinExpirySeconds > 0 {
		cutoff := time.Now().UTC().Add(time.Duration(withinExpirySeconds) * time.Second)
		q = q.Where("expires_at IS NULL OR expires_at <= ?", cutoff)
	}
	if max <= 0 || max > 20000 {
		max = 20000
	}
	var rows []*model.PoolGpt
	if err := q.Order("expires_at ASC").Limit(max).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// PickUsable 选一个可用账号（有 access_token）。
// id>0 时取指定账号；否则取 valid + 未过期的，按使用次数升序（负载均衡）。
// exclude 中的账号会被跳过（用于出图失败后切号）。
func (r *PoolGptRepo) PickUsable(ctx context.Context, id uint64, exclude ...uint64) (*model.PoolGpt, error) {
	q := r.base(ctx).Where("access_token_enc IS NOT NULL")
	if id > 0 {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("status = ?", model.GPTStatusValid).
			Where("expires_at IS NULL OR expires_at > ?", time.Now().UTC()).
			Order("success_count ASC, expires_at DESC")
		if len(exclude) > 0 {
			q = q.Where("id NOT IN ?", exclude)
		}
	}
	var row model.PoolGpt
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// PickUsableRandom 在 valid + 未过期 + 有 access_token 的账号里随机挑一个，跳过 exclude。
func (r *PoolGptRepo) PickUsableRandom(ctx context.Context, exclude []uint64) (*model.PoolGpt, error) {
	q := r.base(ctx).Where("access_token_enc IS NOT NULL").
		Where("status = ?", model.GPTStatusValid).
		Where("expires_at IS NULL OR expires_at > ?", time.Now().UTC()).
		Order("RANDOM()")
	if len(exclude) > 0 {
		q = q.Where("id NOT IN ?", exclude)
	}
	var row model.PoolGpt
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// CountUsable 统计可用账号总数（valid + 未过期 + 有 access_token），用于判断「无号」还是「全忙」。
func (r *PoolGptRepo) CountUsable(ctx context.Context) (int64, error) {
	var n int64
	err := r.base(ctx).Where("access_token_enc IS NOT NULL").
		Where("status = ?", model.GPTStatusValid).
		Where("expires_at IS NULL OR expires_at > ?", time.Now().UTC()).
		Count(&n).Error
	return n, err
}

// IncrSuccess 使用成功后累加计数 + 更新最近使用时间。
func (r *PoolGptRepo) IncrSuccess(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.PoolGpt{}).Where("id = ?", id).Updates(map[string]any{
		"success_count": gorm.Expr("success_count + 1"),
		"last_used_at":  time.Now().UTC(),
	}).Error
}

// GetByID 主键查询。
func (r *PoolGptRepo) GetByID(ctx context.Context, id uint64) (*model.PoolGpt, error) {
	var row model.PoolGpt
	err := r.base(ctx).Where("id = ?", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// Create 插入一条新号。
func (r *PoolGptRepo) Create(ctx context.Context, p *model.PoolGpt) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// Update 字段更新。
func (r *PoolGptRepo) Update(ctx context.Context, id uint64, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.PoolGpt{}).Where("id = ?", id).Updates(fields).Error
}

// SoftDelete 硬删（与上游语义一致）。
func (r *PoolGptRepo) SoftDelete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.PoolGpt{}).Error
}

// SoftDeleteByIDs 批量硬删。
func (r *PoolGptRepo) SoftDeleteByIDs(ctx context.Context, ids []uint64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.PoolGpt{})
	return res.RowsAffected, res.Error
}

// ListForExport 导出（valid / invalid / selected / all）。
func (r *PoolGptRepo) ListForExport(ctx context.Context, scope string, ids []uint64, max int) ([]*model.PoolGpt, error) {
	q := r.base(ctx)
	switch scope {
	case "valid":
		q = q.Where("status = ?", model.GPTStatusValid)
	case "invalid":
		q = q.Where("status <> ?", model.GPTStatusValid)
	case "selected":
		if len(ids) == 0 {
			return nil, nil
		}
		q = q.Where("id IN ?", ids)
	}
	if max <= 0 || max > 20000 {
		max = 20000
	}
	var rows []*model.PoolGpt
	if err := q.Order("id ASC").Limit(max).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
