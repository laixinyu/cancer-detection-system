package repository

// 文件： internal/repository/ops.go
// 用途：仓储层，负责数据访问与持久化。

import (
	"context"
	"time"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OpsRepository interface {
	DBReady(ctx context.Context) error
	CountOpenIncidents(ctx context.Context) (int64, error)
	CountOpenP0P1Incidents(ctx context.Context) (int64, error)
	ListIncidents(ctx context.Context, status string, limit int) ([]domain.OpsIncident, error)
	GetIncidentByID(ctx context.Context, id string) (*domain.OpsIncident, error)
	CreateIncident(ctx context.Context, row *domain.OpsIncident) error
	SaveIncident(ctx context.Context, row *domain.OpsIncident) error
	CreateEvidence(ctx context.Context, row *domain.ClinicalEvidenceRun) error
	UpsertPatientConsent(ctx context.Context, row *domain.PatientConsent) error
	GetPatientByUserID(ctx context.Context, userID string) (*domain.Patient, error)
	CreatePatient(ctx context.Context, row *domain.Patient) error
}

type gormOpsRepository struct {
	db *gorm.DB
}

func NewGormOpsRepository(db *gorm.DB) OpsRepository {
	return &gormOpsRepository{db: db}
}

func (r *gormOpsRepository) DBReady(ctx context.Context) error {
	var n int64
	return r.db.WithContext(ctx).Model(&domain.User{}).Limit(1).Count(&n).Error
}

func (r *gormOpsRepository) CountOpenIncidents(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.OpsIncident{}).Where("status IN ?", []string{"OPEN", "ACKNOWLEDGED"}).Count(&cnt).Error
	return cnt, err
}

func (r *gormOpsRepository) CountOpenP0P1Incidents(ctx context.Context) (int64, error) {
	var cnt int64
	err := r.db.WithContext(ctx).Model(&domain.OpsIncident{}).
		Where("status IN ?", []string{"OPEN", "ACKNOWLEDGED"}).
		Where("severity IN ?", []string{"P0", "P1"}).
		Count(&cnt).Error
	return cnt, err
}

func (r *gormOpsRepository) ListIncidents(ctx context.Context, status string, limit int) ([]domain.OpsIncident, error) {
	q := r.db.WithContext(ctx).
		Model(&domain.OpsIncident{}).
		Preload("Owner").
		Order("opened_at DESC").
		Limit(limit)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var rows []domain.OpsIncident
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *gormOpsRepository) GetIncidentByID(ctx context.Context, id string) (*domain.OpsIncident, error) {
	var row domain.OpsIncident
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormOpsRepository) CreateIncident(ctx context.Context, row *domain.OpsIncident) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *gormOpsRepository) SaveIncident(ctx context.Context, row *domain.OpsIncident) error {
	return r.db.WithContext(ctx).Save(row).Error
}

func (r *gormOpsRepository) CreateEvidence(ctx context.Context, row *domain.ClinicalEvidenceRun) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *gormOpsRepository) UpsertPatientConsent(ctx context.Context, row *domain.PatientConsent) error {
	row.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "patient_id"}, {Name: "consent_type"}, {Name: "consent_version"}},
		DoUpdates: clause.AssignmentColumns([]string{"accepted", "accepted_at", "accepted_by_user_id", "updated_at"}),
	}).Create(row).Error
}

func (r *gormOpsRepository) GetPatientByUserID(ctx context.Context, userID string) (*domain.Patient, error) {
	var row domain.Patient
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormOpsRepository) CreatePatient(ctx context.Context, row *domain.Patient) error {
	return r.db.WithContext(ctx).Create(row).Error
}
