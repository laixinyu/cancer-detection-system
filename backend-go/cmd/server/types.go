package main

import (
	"log/slog"
	"net/http"
	"time"

	"cancer-detection-backend/internal/cache"
	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/service"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

const (
	maxUploadBytes = 10 * 1024 * 1024
)

type app struct {
	db                   *pgxpool.Pool
	orm                  *gorm.DB
	evidenceStore        repository.EvidenceStore
	auditService         *service.AuditService
	authService          *service.AuthService
	detectionService     *service.DetectionService
	reportService        *service.ReportService
	analyticsService     *service.AnalyticsService
	opsService           *service.OpsService
	uploadService        *service.UploadService
	metrics              *observability.Registry
	logger               *slog.Logger
	slowRequestThreshold time.Duration
	cache                cache.Cache
	jwtSecret            []byte
	aiService            string
	detectionServiceURL  string
	reportServiceURL     string
	governanceServiceURL string
	uploadDir            string
	uploadPrefix         string
	httpClient           *http.Client
}

type authClaims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

type userDTO struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Phone    string `json:"phone"`
}

type aiResponse struct {
	ModelVersion            string             `json:"modelVersion"`
	CancerProbability       float64            `json:"cancerProbability"`
	Regions                 []map[string]any   `json:"regions"`
	LabelScores             map[string]float64 `json:"labelScores"`
	InfectionCoverage       map[string]float64 `json:"infectionCoverage"`
	WhiteLungAssessment     map[string]any     `json:"whiteLungAssessment"`
	TopFindings             []string           `json:"topFindings"`
	CalibrationTemperature  *float64           `json:"calibrationTemperature"`
	DecisionHighSensitivity *bool              `json:"decisionHighSensitivity"`
	DecisionHighSpecificity *bool              `json:"decisionHighSpecificity"`
	TaskDecisions           map[string]any     `json:"taskDecisions"`
	OperatingPointsUsed     map[string]any     `json:"operatingPointsUsed"`
	ClinicalUse             string             `json:"clinicalUse"`
	ClinicalStage           string             `json:"clinicalStage"`
	DetectorModelLoaded     *bool              `json:"detectorModelLoaded"`
	HeatmapPath             *string            `json:"heatmapPath"`
}

type imageResponse struct {
	ID           string `json:"id"`
	OriginalName string `json:"originalName"`
	FilePath     string `json:"filePath"`
	Status       string `json:"status"`
}
