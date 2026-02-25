package gormdb

// 文件： internal/platform/gormdb/gormdb.go
// 用途：平台集成辅助组件（如数据库初始化）。

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open gorm db: %w", err)
	}
	return db, nil
}
