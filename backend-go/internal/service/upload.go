package service

// File: internal/service/upload.go
// Purpose: Service layer containing business rules and orchestration logic.

import (
	"context"
	"errors"
	"strings"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"
)

type UploadService struct {
	repo repository.UploadRepository
	ops  *OpsService
}

func NewUploadService(repo repository.UploadRepository, ops *OpsService) *UploadService {
	return &UploadService{repo: repo, ops: ops}
}

func (s *UploadService) ListImages(ctx context.Context, role, userID string, limit int) ([]domain.Image, *string, error) {
	if limit <= 0 {
		limit = 20
	}
	uploadedBy := ""
	if role == "PATIENT" {
		uploadedBy = userID
	}
	rows, err := s.repo.ListImages(ctx, uploadedBy, limit+1)
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

func (s *UploadService) FindOrCreatePatient(ctx context.Context, userID, role string) (string, error) {
	return s.ops.FindOrCreatePatient(ctx, userID, role)
}

func (s *UploadService) SaveConsent(ctx context.Context, row *domain.PatientConsent) error {
	return s.ops.UpsertConsent(ctx, row)
}

func (s *UploadService) CreateImage(ctx context.Context, row *domain.Image) error {
	return s.repo.CreateImage(ctx, row)
}

func (s *UploadService) MarkImageFailed(ctx context.Context, id string) error {
	return s.repo.UpdateImageStatus(ctx, id, "FAILED")
}

func (s *UploadService) MarkImageCompleted(ctx context.Context, id string) error {
	return s.repo.UpdateImageStatus(ctx, id, "COMPLETED")
}

func (s *UploadService) CreateDetection(ctx context.Context, row *domain.Detection) error {
	return s.repo.CreateDetection(ctx, row)
}

func (s *UploadService) GetImageFileMeta(ctx context.Context, id, role, userID, uploadPrefix string) (*domain.Image, error) {
	img, err := s.repo.GetImageByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role == "PATIENT" && img.UploadedBy != userID {
		return nil, errors.New("forbidden")
	}
	if !strings.HasPrefix(img.FilePath, uploadPrefix+"/") {
		return nil, errors.New("unsupported storage path")
	}
	return img, nil
}
