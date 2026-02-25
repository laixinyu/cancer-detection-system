package service

// 文件： internal/service/detection.go
// 用途：服务层，承载业务规则与流程编排逻辑。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"
)

var (
	ErrInvalidDetectionStatus     = errors.New("invalid status")
	ErrInvalidDetectionTransition = errors.New("invalid detection status transition")
)

type DetectionService struct {
	repo repository.DetectionRepository
}

func NewDetectionService(repo repository.DetectionRepository) *DetectionService {
	return &DetectionService{repo: repo}
}

type ListDetectionsInput struct {
	Status          string
	Priority        string
	OrderByPriority bool
	PatientUserID   string
	Limit           int
}

type ReviewDetectionInput struct {
	ID          string
	Status      string
	ReviewerID  string
	ReviewNotes string
	Findings    any
}

func (s *DetectionService) List(ctx context.Context, in ListDetectionsInput) ([]domain.Detection, *string, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.repo.List(ctx, repository.DetectionListFilter{
		Status:          strings.TrimSpace(in.Status),
		Priority:        strings.TrimSpace(in.Priority),
		OrderByPriority: in.OrderByPriority,
		PatientUserID:   strings.TrimSpace(in.PatientUserID),
		Limit:           limit + 1,
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

func (s *DetectionService) GetByID(ctx context.Context, id string) (*domain.Detection, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *DetectionService) Review(ctx context.Context, in ReviewDetectionInput) (*domain.Detection, string, error) {
	status := strings.TrimSpace(in.Status)
	if status != "REVIEWED" && status != "CONFIRMED" {
		return nil, "", ErrInvalidDetectionStatus
	}
	det, err := s.repo.GetByID(ctx, in.ID)
	if err != nil {
		return nil, "", err
	}
	oldStatus := det.Status
	if !(oldStatus == "PENDING" && (status == "REVIEWED" || status == "CONFIRMED")) &&
		!(oldStatus == "REVIEWED" && status == "CONFIRMED") {
		return nil, "", ErrInvalidDetectionTransition
	}

	var findings []byte
	hasFindings := false
	if in.Findings != nil {
		findings, _ = json.Marshal(in.Findings)
		hasFindings = true
	}
	var notes *string
	if trimmed := strings.TrimSpace(in.ReviewNotes); trimmed != "" {
		notes = &trimmed
	}

	if err := s.repo.UpdateReview(ctx, in.ID, status, in.ReviewerID, notes, findings, hasFindings); err != nil {
		return nil, "", err
	}
	updated, err := s.repo.GetByID(ctx, in.ID)
	if err != nil {
		return nil, "", err
	}
	return updated, oldStatus, nil
}
