package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"

	"gorm.io/gorm"
)

var (
	ErrInvalidIncidentStatus = errors.New("invalid status")
	ErrInvalidIncidentPayload = errors.New("invalid incident payload")
	ErrInvalidIncidentTransition = errors.New("invalid incident transition")
)

type OpsService struct {
	repo repository.OpsRepository
}

func NewOpsService(repo repository.OpsRepository) *OpsService {
	return &OpsService{repo: repo}
}

func (s *OpsService) DBReady(ctx context.Context) error {
	return s.repo.DBReady(ctx)
}

func (s *OpsService) DashboardCounts(ctx context.Context) (int, int, error) {
	openCnt, err := s.repo.CountOpenIncidents(ctx)
	if err != nil {
		return 0, 0, err
	}
	p0p1Cnt, err := s.repo.CountOpenP0P1Incidents(ctx)
	if err != nil {
		return 0, 0, err
	}
	return int(openCnt), int(p0p1Cnt), nil
}

func (s *OpsService) ListIncidents(ctx context.Context, status string, limit int) ([]domain.OpsIncident, error) {
	status = strings.TrimSpace(status)
	if status != "" && status != "OPEN" && status != "ACKNOWLEDGED" && status != "RESOLVED" {
		return nil, ErrInvalidIncidentStatus
	}
	if limit <= 0 {
		limit = 30
	}
	return s.repo.ListIncidents(ctx, status, limit)
}

type CreateIncidentInput struct {
	Source      string
	Severity    string
	Title       string
	Detail      string
	OwnerUserID string
}

func (s *OpsService) CreateIncident(ctx context.Context, in CreateIncidentInput) (*domain.OpsIncident, error) {
	in.Source = strings.TrimSpace(in.Source)
	in.Severity = strings.TrimSpace(in.Severity)
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || !containsString([]string{"AI_SERVICE", "APP", "DATABASE", "PIPELINE", "SECURITY", "OTHER"}, in.Source) || !containsString([]string{"P0", "P1", "P2", "P3"}, in.Severity) {
		return nil, ErrInvalidIncidentPayload
	}
	row := &domain.OpsIncident{
		Source:      in.Source,
		Severity:    in.Severity,
		Status:      "OPEN",
		Title:       in.Title,
		Detail:      strPtr(in.Detail),
		OwnerUserID: strPtr(in.OwnerUserID),
		OpenedAt:    time.Now().UTC(),
	}
	if err := s.repo.CreateIncident(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *OpsService) TransitionIncident(ctx context.Context, incidentID, action, actorUserID string) (*domain.OpsIncident, string, error) {
	incidentID = strings.TrimSpace(incidentID)
	action = strings.TrimSpace(action)
	if incidentID == "" || !containsString([]string{"ACKNOWLEDGE", "RESOLVE", "REOPEN"}, action) {
		return nil, "", ErrInvalidIncidentTransition
	}
	row, err := s.repo.GetIncidentByID(ctx, incidentID)
	if err != nil {
		return nil, "", err
	}
	old := row.Status
	now := time.Now().UTC()
	switch action {
	case "ACKNOWLEDGE":
		if row.OwnerUserID == nil || *row.OwnerUserID == "" {
			row.OwnerUserID = &actorUserID
		}
		row.Status = "ACKNOWLEDGED"
		row.AcknowledgedAt = &now
		row.UpdatedAt = now
	case "RESOLVE":
		row.Status = "RESOLVED"
		row.ResolvedAt = &now
		row.UpdatedAt = now
	case "REOPEN":
		row.Status = "OPEN"
		row.AcknowledgedAt = nil
		row.ResolvedAt = nil
		row.UpdatedAt = now
	}
	if err := s.repo.SaveIncident(ctx, row); err != nil {
		return nil, "", err
	}
	return row, old, nil
}

func (s *OpsService) CreateEvidence(ctx context.Context, row *domain.ClinicalEvidenceRun) error {
	return s.repo.CreateEvidence(ctx, row)
}

func (s *OpsService) FindOrCreatePatient(ctx context.Context, userID, role string) (string, error) {
	patient, err := s.repo.GetPatientByUserID(ctx, userID)
	if err == nil {
		return patient.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("failed to query patient profile")
	}
	if role != "PATIENT" {
		return "", errors.New("patient profile not found")
	}
	patient = &domain.Patient{
		UserID:      userID,
		DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Gender:      "OTHER",
	}
	if err := s.repo.CreatePatient(ctx, patient); err != nil {
		return "", errors.New("patient profile not found")
	}
	return patient.ID, nil
}

func (s *OpsService) UpsertConsent(ctx context.Context, row *domain.PatientConsent) error {
	return s.repo.UpsertPatientConsent(ctx, row)
}

func strPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

