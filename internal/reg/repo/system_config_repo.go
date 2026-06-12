package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// SystemConfigKV 系统配置键值表。value 存 JSON 文本或裸字符串。
type SystemConfigKV struct {
	Key       string    `gorm:"primaryKey;column:k;size:128" json:"k"`
	Value     string    `gorm:"column:v;type:text" json:"v"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName 表名。
func (SystemConfigKV) TableName() string { return "system_config" }

// SystemConfigRepo 系统配置键值仓库。
type SystemConfigRepo struct{ db *gorm.DB }

// NewSystemConfigRepo 构造。
func NewSystemConfigRepo(db *gorm.DB) *SystemConfigRepo { return &SystemConfigRepo{db: db} }

// Get 读取一个 key 的原始字符串值；不存在返回 ErrNotFound。
func (r *SystemConfigRepo) Get(ctx context.Context, key string) (string, error) {
	var row SystemConfigKV
	err := r.db.WithContext(ctx).Where("k = ?", key).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return row.Value, nil
}

// Set 写入一个 key（upsert）。
func (r *SystemConfigRepo) Set(ctx context.Context, key, value string) error {
	row := SystemConfigKV{Key: key, Value: value, UpdatedAt: time.Now().UTC()}
	return r.db.WithContext(ctx).Save(&row).Error
}

// All 返回全部配置（管理界面用）。
func (r *SystemConfigRepo) All(ctx context.Context) (map[string]string, error) {
	var rows []SystemConfigKV
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}
