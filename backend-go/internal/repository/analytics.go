package repository

// 文件： internal/repository/analytics.go
// 用途：仓储层，负责数据访问与持久化。

import (
	"context"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
)

type AnalyticsRepository interface {
	CountUsers(ctx context.Context) (int64, error)
	CountImages(ctx context.Context) (int64, error)
	CountDetections(ctx context.Context) (int64, error)
	CountReports(ctx context.Context) (int64, error)
	CountUsersByRole(ctx context.Context, role string) (int64, error)
	CountDetectionsByStatus(ctx context.Context, status string) (int64, error)
	CountReportsByStatus(ctx context.Context, status string) (int64, error)
	ListRecentUsers(ctx context.Context, limit int) ([]domain.User, error)
	ListRecentAuditLogs(ctx context.Context, limit int) ([]domain.AuditLog, error)
}

type gormAnalyticsRepository struct {
	db *gorm.DB
}

func NewGormAnalyticsRepository(db *gorm.DB) AnalyticsRepository {
	return &gormAnalyticsRepository{db: db}
}

func (r *gormAnalyticsRepository) CountUsers(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.User{}).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) CountImages(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.Image{}).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) CountDetections(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.Detection{}).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) CountReports(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.Report{}).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) CountUsersByRole(ctx context.Context, role string) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.User{}).Where("role = ?", role).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) CountDetectionsByStatus(ctx context.Context, status string) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.Detection{}).Where("status = ?", status).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) CountReportsByStatus(ctx context.Context, status string) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.Report{}).Where("status = ?", status).Count(&cnt).Error
	return cnt, err
}

func (r *gormAnalyticsRepository) ListRecentUsers(ctx context.Context, limit int) ([]domain.User, error) {
	var rows []domain.User
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *gormAnalyticsRepository) ListRecentAuditLogs(ctx context.Context, limit int) ([]domain.AuditLog, error) {
	var rows []domain.AuditLog
	err := r.db.WithContext(ctx).Preload("ActorUser").Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}
