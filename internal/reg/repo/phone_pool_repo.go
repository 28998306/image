package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"web2img/internal/reg/model"
)

// PhonePoolRepo 接码手机号池仓库。
type PhonePoolRepo struct{ db *gorm.DB }

// NewPhonePoolRepo 构造。
func NewPhonePoolRepo(db *gorm.DB) *PhonePoolRepo { return &PhonePoolRepo{db: db} }

// GetByID 主键查询。
func (r *PhonePoolRepo) GetByID(ctx context.Context, id uint64) (*model.PhonePool, error) {
	var row model.PhonePool
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// AcquireOrInsert 复用一条可用号；没有则用 fallback 插入新号。
//
// fallback 为 nil 时只尝试复用（找不到返回 ErrNotFound）。
func (r *PhonePoolRepo) AcquireOrInsert(ctx context.Context, provider, service string, countries []int, fallback *model.PhonePool) (*model.PhonePool, error) {
	var out *model.PhonePool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&model.PhonePool{}).
			Where("deleted_at IS NULL AND provider = ? AND status = ? AND used_count < max_uses", provider, model.PhoneStatusAvailable)
		if len(countries) > 0 {
			q = q.Where("country IN ?", countries)
		}
		var row model.PhonePool
		e := q.Order("used_count DESC, id ASC").First(&row).Error
		if e == nil {
			now := time.Now().UTC()
			if err := tx.Model(&model.PhonePool{}).Where("id = ?", row.ID).Updates(map[string]any{
				"status":       model.PhoneStatusInUse,
				"last_used_at": now,
			}).Error; err != nil {
				return err
			}
			row.Status = model.PhoneStatusInUse
			out = &row
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if fallback == nil {
			return ErrNotFound
		}
		fallback.Provider = provider
		if fallback.Service == "" {
			fallback.Service = service
		}
		fallback.Status = model.PhoneStatusInUse
		if fallback.MaxUses <= 0 {
			fallback.MaxUses = 3
		}
		if err := tx.Create(fallback).Error; err != nil {
			return err
		}
		out = fallback
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateActivationID 更新激活 ID（复用同号新申请）。
func (r *PhonePoolRepo) UpdateActivationID(ctx context.Context, id uint64, activationID string) error {
	return r.db.WithContext(ctx).Model(&model.PhonePool{}).Where("id = ?", id).
		Update("activation_id", activationID).Error
}

// Release 归还为可用。
func (r *PhonePoolRepo) Release(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Model(&model.PhonePool{}).
		Where("id = ? AND status = ?", id, model.PhoneStatusInUse).
		Update("status", model.PhoneStatusAvailable).Error
}

// MarkVerified 验证成功：used_count++，达上限置 exhausted。
func (r *PhonePoolRepo) MarkVerified(ctx context.Context, id, accountID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.PhonePool
		if e := tx.Where("id = ?", id).First(&row).Error; e != nil {
			return e
		}
		row.UsedCount++
		now := time.Now().UTC()
		fields := map[string]any{
			"used_count":   row.UsedCount,
			"last_used_at": now,
			"status":       model.PhoneStatusAvailable,
		}
		if accountID > 0 {
			fields["last_account_id"] = accountID
		}
		if row.UsedCount >= row.MaxUses {
			fields["status"] = model.PhoneStatusExhausted
		}
		return tx.Model(&model.PhonePool{}).Where("id = ?", id).Updates(fields).Error
	})
}

// MarkFailed 失败计数 +1；达上限置 broken。
func (r *PhonePoolRepo) MarkFailed(ctx context.Context, id uint64, reason string, maxFailure int) error {
	if maxFailure <= 0 {
		maxFailure = 2
	}
	if len(reason) > 240 {
		reason = reason[:240]
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.PhonePool
		if e := tx.Where("id = ?", id).First(&row).Error; e != nil {
			return e
		}
		row.FailureCount++
		fields := map[string]any{
			"failure_count": row.FailureCount,
			"last_error":    reason,
		}
		if row.FailureCount >= maxFailure {
			fields["status"] = model.PhoneStatusBroken
		} else {
			fields["status"] = model.PhoneStatusAvailable
		}
		return tx.Model(&model.PhonePool{}).Where("id = ?", id).Updates(fields).Error
	})
}
