package repository

// File: internal/repository/evidence.go
// Purpose: Repository layer responsible for data access and persistence.

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type UserBrief struct {
	ID    string
	Name  string
	Email string
}

type EvidenceRecord struct {
	ID                       string
	RunName                  string
	ModelVersion             string
	DatasetName              string
	DatasetVersion           *string
	SampleCount              int
	PositiveCount            int
	SiteCount                int
	Auroc                    float64
	Sensitivity              float64
	Specificity              float64
	PPV                      *float64
	NPV                      *float64
	ECE                      *float64
	Brier                    *float64
	CalibrationTemperature   *float64
	ThresholdHighSensitivity *float64
	ThresholdHighSpecificity *float64
	RegulatoryStatus         string
	StageRecommendation      string
	QAApprovedBy             *string
	MedicalApprovedBy        *string
	ReportPath               *string
	Notes                    *string
	CreatedByUserID          *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
	CreatedBy                *UserBrief
}

type EvidenceStore interface {
	ListEvidence(ctx context.Context, limit int) ([]EvidenceRecord, error)
	GetLatestEvidence(ctx context.Context) (*EvidenceRecord, error)
}

type gormEvidenceStore struct {
	db *gorm.DB
}

func NewGormEvidenceStore(db *gorm.DB) EvidenceStore {
	return &gormEvidenceStore{db: db}
}

type clinicalEvidenceRunModel struct {
	ID                       string     `gorm:"column:id"`
	RunName                  string     `gorm:"column:run_name"`
	ModelVersion             string     `gorm:"column:model_version"`
	DatasetName              string     `gorm:"column:dataset_name"`
	DatasetVersion           *string    `gorm:"column:dataset_version"`
	SampleCount              int        `gorm:"column:sample_count"`
	PositiveCount            int        `gorm:"column:positive_count"`
	SiteCount                int        `gorm:"column:site_count"`
	Auroc                    float64    `gorm:"column:auroc"`
	Sensitivity              float64    `gorm:"column:sensitivity"`
	Specificity              float64    `gorm:"column:specificity"`
	PPV                      *float64   `gorm:"column:ppv"`
	NPV                      *float64   `gorm:"column:npv"`
	ECE                      *float64   `gorm:"column:ece"`
	Brier                    *float64   `gorm:"column:brier"`
	CalibrationTemperature   *float64   `gorm:"column:calibration_temperature"`
	ThresholdHighSensitivity *float64   `gorm:"column:threshold_high_sensitivity"`
	ThresholdHighSpecificity *float64   `gorm:"column:threshold_high_specificity"`
	RegulatoryStatus         string     `gorm:"column:regulatory_status"`
	StageRecommendation      string     `gorm:"column:stage_recommendation"`
	QAApprovedBy             *string    `gorm:"column:qa_approved_by"`
	MedicalApprovedBy        *string    `gorm:"column:medical_approved_by"`
	ReportPath               *string    `gorm:"column:report_path"`
	Notes                    *string    `gorm:"column:notes"`
	CreatedByUserID          *string    `gorm:"column:created_by_user_id"`
	CreatedAt                time.Time  `gorm:"column:created_at"`
	UpdatedAt                time.Time  `gorm:"column:updated_at"`
	CreatedBy                *userModel `gorm:"foreignKey:CreatedByUserID;references:ID"`
}

func (clinicalEvidenceRunModel) TableName() string {
	return "clinical_evidence_runs"
}

type userModel struct {
	ID    string `gorm:"column:id"`
	Name  string `gorm:"column:name"`
	Email string `gorm:"column:email"`
}

func (userModel) TableName() string {
	return "users"
}

func (s *gormEvidenceStore) ListEvidence(ctx context.Context, limit int) ([]EvidenceRecord, error) {
	var rows []clinicalEvidenceRunModel
	if err := s.db.WithContext(ctx).
		Model(&clinicalEvidenceRunModel{}).
		Preload("CreatedBy").
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]EvidenceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEvidenceRecord(row))
	}
	return out, nil
}

func (s *gormEvidenceStore) GetLatestEvidence(ctx context.Context) (*EvidenceRecord, error) {
	var row clinicalEvidenceRunModel
	err := s.db.WithContext(ctx).
		Model(&clinicalEvidenceRunModel{}).
		Preload("CreatedBy").
		Order("created_at DESC").
		First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	rec := toEvidenceRecord(row)
	return &rec, nil
}

func toEvidenceRecord(row clinicalEvidenceRunModel) EvidenceRecord {
	var createdBy *UserBrief
	if row.CreatedBy != nil {
		createdBy = &UserBrief{
			ID:    row.CreatedBy.ID,
			Name:  row.CreatedBy.Name,
			Email: row.CreatedBy.Email,
		}
	}

	return EvidenceRecord{
		ID:                       row.ID,
		RunName:                  row.RunName,
		ModelVersion:             row.ModelVersion,
		DatasetName:              row.DatasetName,
		DatasetVersion:           row.DatasetVersion,
		SampleCount:              row.SampleCount,
		PositiveCount:            row.PositiveCount,
		SiteCount:                row.SiteCount,
		Auroc:                    row.Auroc,
		Sensitivity:              row.Sensitivity,
		Specificity:              row.Specificity,
		PPV:                      row.PPV,
		NPV:                      row.NPV,
		ECE:                      row.ECE,
		Brier:                    row.Brier,
		CalibrationTemperature:   row.CalibrationTemperature,
		ThresholdHighSensitivity: row.ThresholdHighSensitivity,
		ThresholdHighSpecificity: row.ThresholdHighSpecificity,
		RegulatoryStatus:         row.RegulatoryStatus,
		StageRecommendation:      row.StageRecommendation,
		QAApprovedBy:             row.QAApprovedBy,
		MedicalApprovedBy:        row.MedicalApprovedBy,
		ReportPath:               row.ReportPath,
		Notes:                    row.Notes,
		CreatedByUserID:          row.CreatedByUserID,
		CreatedAt:                row.CreatedAt,
		UpdatedAt:                row.UpdatedAt,
		CreatedBy:                createdBy,
	}
}
