package main

import (
	"net/http"

	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/service"
)

type governanceApp struct {
	auditSvc      *service.AuditService
	analyticsSvc  *service.AnalyticsService
	opsSvc        *service.OpsService
	evidenceStore repository.EvidenceStore
	httpClient    *http.Client
	aiServiceURL  string
}
