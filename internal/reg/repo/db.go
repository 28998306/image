// Package repo 注册子系统的数据访问层，基于 GORM + 纯 Go SQLite（glebarez）。
package repo

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"web2img/internal/reg/model"
)

// ErrNotFound 统一的"记录不存在"错误。
var ErrNotFound = errors.New("repo: record not found")

// Open 打开（或创建）SQLite 数据库并自动迁移注册子系统所需表。
func Open(dbPath string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&model.MailPool{},
		&model.RegisterTask{},
		&model.RegisterTaskLog{},
		&model.PoolGpt{},
		&model.PhonePool{},
		&model.Proxy{},
		&SystemConfigKV{},
	); err != nil {
		return nil, err
	}
	return db, nil
}
