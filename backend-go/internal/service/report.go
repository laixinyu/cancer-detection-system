package service

// 文件： internal/service/report.go
// 用途：服务层，承载业务规则与流程编排逻辑。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"

	"gorm.io/gorm"
)

var (
	ErrInvalidReportStatus     = errors.New("invalid report status")
	ErrInvalidReportTransition = errors.New("invalid report status transition")
	ErrInvalidReportPayload    = errors.New("detectionId and patientId are required")
	ErrDetectionNotReviewed    = errors.New("detection must be reviewed before report creation")
	ErrPatientMismatch         = errors.New("patientId does not match detection patient")
	ErrNoReportFieldsToUpdate  = errors.New("no fields to update")
)

type ReportService struct {
	repo repository.ReportRepository
}

func NewReportService(repo repository.ReportRepository) *ReportService {
	return &ReportService{repo: repo}
}

type ListReportsInput struct {
	Status        string
	PatientID     string
	PatientUserID string
	Limit         int
}

type UpsertReportInput struct {
	DetectionID string
	PatientID   string
	DoctorID    string
	Content     any
	Status      string
}

type UpdateReportInput struct {
	ID      string
	Content *any
	Status  string
	PDFPath string
}

func (s *ReportService) List(ctx context.Context, in ListReportsInput) ([]domain.Report, *string, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.repo.List(ctx, repository.ReportListFilter{
		Status:        strings.TrimSpace(in.Status),
		PatientID:     strings.TrimSpace(in.PatientID),
		PatientUserID: strings.TrimSpace(in.PatientUserID),
		Limit:         limit + 1,
	})
	if err != nil {
		return nil, nil, err
	}
	var next *string
	if len(rows) > limit {
		id := rows[len(rows)-1].ID
		next = &id
		rows = rows[:len(rows)-1]
	}
	return rows, next, nil
}

func (s *ReportService) GetByID(ctx context.Context, id string) (*domain.Report, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *ReportService) Upsert(ctx context.Context, in UpsertReportInput) (*domain.Report, bool, error) {
	in.DetectionID = strings.TrimSpace(in.DetectionID)
	in.PatientID = strings.TrimSpace(in.PatientID)
	in.Status = strings.TrimSpace(in.Status)
	if in.Status == "" {
		in.Status = "DRAFT"
	}
	if in.DetectionID == "" || in.PatientID == "" {
		return nil, false, ErrInvalidReportPayload
	}
	if in.Status != "DRAFT" && in.Status != "FINALIZED" {
		return nil, false, ErrInvalidReportStatus
	}

	detection, err := s.repo.GetDetection(ctx, in.DetectionID)
	if err != nil {
		return nil, false, err
	}
	if detection.Status != "REVIEWED" && detection.Status != "CONFIRMED" {
		return nil, false, ErrDetectionNotReviewed
	}
	if detection.Image.PatientID != in.PatientID {
		return nil, false, ErrPatientMismatch
	}

	content := in.Content
	if content == nil {
		content = map[string]any{}
	}
	contentBytes, _ := json.Marshal(content)

	existing, err := s.repo.GetByDetectionID(ctx, in.DetectionID)
	if err == nil {
		if err := s.repo.Update(ctx, existing.ID, map[string]any{
			"content":    contentBytes,
			"status":     in.Status,
			"patient_id": in.PatientID,
			"doctor_id":  in.DoctorID,
		}); err != nil {
			return nil, false, err
		}
		updated, err := s.repo.GetByID(ctx, existing.ID)
		if err != nil {
			return nil, false, err
		}
		return updated, true, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	row := &domain.Report{
		DetectionID: in.DetectionID,
		PatientID:   in.PatientID,
		DoctorID:    in.DoctorID,
		Content:     contentBytes,
		Status:      in.Status,
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, false, err
	}
	created, err := s.repo.GetByID(ctx, row.ID)
	if err != nil {
		return nil, false, err
	}
	return created, false, nil
}

func (s *ReportService) Update(ctx context.Context, in UpdateReportInput) (*domain.Report, string, []string, error) {
	row, err := s.repo.GetByID(ctx, in.ID)
	if err != nil {
		return nil, "", nil, err
	}
	oldStatus := row.Status

	newStatus := strings.TrimSpace(in.Status)
	if newStatus != "" {
		if newStatus != "DRAFT" && newStatus != "FINALIZED" {
			return nil, "", nil, ErrInvalidReportStatus
		}
		if oldStatus == "FINALIZED" && newStatus != "FINALIZED" {
			return nil, "", nil, ErrInvalidReportTransition
		}
	}

	updates := map[string]any{}
	updatedFields := make([]string, 0, 3)
	if in.Content != nil {
		b, _ := json.Marshal(*in.Content)
		updates["content"] = b
		updatedFields = append(updatedFields, "content")
	}
	if newStatus != "" {
		updates["status"] = newStatus
		updatedFields = append(updatedFields, "status")
	}
	if p := strings.TrimSpace(in.PDFPath); p != "" {
		updates["pdf_path"] = p
		updatedFields = append(updatedFields, "pdfPath")
	}
	if len(updatedFields) == 0 {
		return nil, "", nil, ErrNoReportFieldsToUpdate
	}
	if err := s.repo.Update(ctx, in.ID, updates); err != nil {
		return nil, "", nil, err
	}
	updated, err := s.repo.GetByID(ctx, in.ID)
	if err != nil {
		return nil, "", nil, err
	}
	return updated, oldStatus, updatedFields, nil
}
