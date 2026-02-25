package gormdb

// File: internal/platform/gormdb/gormdb.go
// Purpose: Platform integration helpers such as database bootstrapping.

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
