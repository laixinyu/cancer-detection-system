package service

// 文件： internal/service/analytics.go
// 用途：服务层，承载业务规则与流程编排逻辑。

import (
	"context"

	"cancer-detection-backend/internal/repository"
)

type AnalyticsService struct {
	repo repository.AnalyticsRepository
}

func NewAnalyticsService(repo repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

type AdminOverviewOutput struct {
	Users             int
	Doctors           int
	Patients          int
	Images            int
	Detections        int
	PendingDetections int
	Reports           int
	FinalizedReports  int
	RecentUsers       []map[string]any
	RecentActivity    []map[string]any
}

func (s *AnalyticsService) AdminOverview(ctx context.Context) (*AdminOverviewOutput, error) {
	usersCount, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, err
	}
	imagesCount, err := s.repo.CountImages(ctx)
	if err != nil {
		return nil, err
	}
	detectionsCount, err := s.repo.CountDetections(ctx)
	if err != nil {
		return nil, err
	}
	reportsCount, err := s.repo.CountReports(ctx)
	if err != nil {
		return nil, err
	}
	doctorCount, err := s.repo.CountUsersByRole(ctx, "DOCTOR")
	if err != nil {
		return nil, err
	}
	patientCount, err := s.repo.CountUsersByRole(ctx, "PATIENT")
	if err != nil {
		return nil, err
	}
	pendingDetections, err := s.repo.CountDetectionsByStatus(ctx, "PENDING")
	if err != nil {
		return nil, err
	}
	finalizedReports, err := s.repo.CountReportsByStatus(ctx, "FINALIZED")
	if err != nil {
		return nil, err
	}

	recentUsersRows, err := s.repo.ListRecentUsers(ctx, 5)
	if err != nil {
		return nil, err
	}
	recentUsers := make([]map[string]any, 0, len(recentUsersRows))
	for _, u := range recentUsersRows {
		recentUsers = append(recentUsers, map[string]any{
			"id":        u.ID,
			"name":      u.Name,
			"email":     u.Email,
			"role":      u.Role,
			"createdAt": u.CreatedAt,
		})
	}

	recentAuditRows, err := s.repo.ListRecentAuditLogs(ctx, 10)
	if err != nil {
		return nil, err
	}
	recentActivity := make([]map[string]any, 0, len(recentAuditRows))
	for _, aLog := range recentAuditRows {
		var actorUser any = nil
		if aLog.ActorUser != nil {
			actorUser = map[string]any{
				"id":    aLog.ActorUser.ID,
				"name":  aLog.ActorUser.Name,
				"email": aLog.ActorUser.Email,
				"role":  aLog.ActorUser.Role,
			}
		}
		recentActivity = append(recentActivity, map[string]any{
			"id":         aLog.ID,
			"action":     aLog.Action,
			"entityType": aLog.EntityType,
			"entityId":   aLog.EntityID,
			"result":     aLog.Result,
			"metadata":   decodeMetadata(aLog.Metadata),
			"createdAt":  aLog.CreatedAt,
			"actorUser":  actorUser,
		})
	}

	return &AdminOverviewOutput{
		Users:             int(usersCount),
		Doctors:           int(doctorCount),
		Patients:          int(patientCount),
		Images:            int(imagesCount),
		Detections:        int(detectionsCount),
		PendingDetections: int(pendingDetections),
		Reports:           int(reportsCount),
		FinalizedReports:  int(finalizedReports),
		RecentUsers:       recentUsers,
		RecentActivity:    recentActivity,
	}, nil
}
