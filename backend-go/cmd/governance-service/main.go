package main

// File: cmd/governance-service/main.go
// Purpose: Governance microservice entrypoint and audit/analytics/ops handlers.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/platform/gormdb"
	"cancer-detection-backend/internal/platform/lifecycle"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/rpc/bridge"
	"cancer-detection-backend/internal/rpc/bridgehttp"
	rpccodec "cancer-detection-backend/internal/rpc/codec"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

type claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	jwt.RegisteredClaims
}

func main() {
	traceShutdown, err := observability.InitTracingFromEnv(context.Background(), "governance-service")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = traceShutdown(context.Background())
	}()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8083"
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := strings.TrimSpace(os.Getenv("BACKEND_JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}
	aiServiceURL := strings.TrimRight(envOr("AI_SERVICE_URL", "http://localhost:8000"), "/")

	gdb, err := gormdb.Open(dsn)
	if err != nil {
		log.Fatalf("init gorm: %v", err)
	}
	httpClient := &http.Client{Timeout: 20 * time.Second}

	auditSvc := service.NewAuditService(repository.NewGormAuditRepository(gdb))
	analyticsSvc := service.NewAnalyticsService(repository.NewGormAnalyticsRepository(gdb))
	opsSvc := service.NewOpsService(repository.NewGormOpsRepository(gdb))
	evidenceStore := repository.NewGormEvidenceStore(gdb)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "governance-service"})
	})

	v1 := r.Group("/api/v1")
	v1.Use(authRequired([]byte(jwtSecret)))
	{
		v1.GET("/audits", func(c *gin.Context) {
			cl := mustClaims(c)
			if cl.Role != "ADMIN" && cl.Role != "DOCTOR" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors and admins can view audit logs"})
				return
			}
			out, err := auditSvc.ListAudits(c.Request.Context(), service.ListAuditsInput{
				Action:     strings.TrimSpace(c.Query("action")),
				EntityType: strings.TrimSpace(c.Query("entityType")),
				EntityID:   strings.TrimSpace(c.Query("entityId")),
				Result:     strings.TrimSpace(c.Query("result")),
				Limit:      parseLimit(c, 20),
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit logs"})
				return
			}
			items := make([]map[string]any, 0, len(out.Logs))
			for _, row := range out.Logs {
				var actorUser any = nil
				if row.ActorUser != nil {
					actorUser = gin.H{"id": row.ActorUser.ID, "name": row.ActorUser.Name, "email": row.ActorUser.Email, "role": row.ActorUser.Role}
				}
				items = append(items, gin.H{
					"id":          row.ID,
					"actorUserId": strOrNil(row.ActorUserID),
					"actorRole":   strOrNil(row.ActorRole),
					"action":      row.Action,
					"entityType":  row.EntityType,
					"entityId":    row.EntityID,
					"result":      row.Result,
					"metadata":    row.Metadata,
					"createdAt":   row.CreatedAt,
					"actorUser":   actorUser,
				})
			}
			var nextCursor any = nil
			if out.NextCursor != nil {
				nextCursor = *out.NextCursor
			}
			c.JSON(http.StatusOK, gin.H{"logs": items, "nextCursor": nextCursor})
		})

		v1.GET("/analytics/admin-overview", func(c *gin.Context) {
			if !assertAdmin(c, mustClaims(c)) {
				return
			}
			out, err := analyticsSvc.AdminOverview(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load analytics"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"stats": gin.H{
					"users":             out.Users,
					"doctors":           out.Doctors,
					"patients":          out.Patients,
					"images":            out.Images,
					"detections":        out.Detections,
					"pendingDetections": out.PendingDetections,
					"reports":           out.Reports,
					"finalizedReports":  out.FinalizedReports,
				},
				"recentUsers":    out.RecentUsers,
				"recentActivity": out.RecentActivity,
			})
		})

		v1.GET("/ops/readiness", func(c *gin.Context) {
			if !assertAdmin(c, mustClaims(c)) {
				return
			}
			dbReady := true
			dbErr := ""
			if err := opsSvc.DBReady(c.Request.Context()); err != nil {
				dbReady = false
				dbErr = err.Error()
			}
			start := time.Now()
			aiReachable := true
			aiStatus := "ok"
			var aiHealth any = nil
			resp, err := httpClient.Get(aiServiceURL + "/health")
			if err != nil {
				aiReachable = false
				aiStatus = err.Error()
			} else {
				defer resp.Body.Close()
				if resp.StatusCode >= 400 {
					aiReachable = false
					aiStatus = fmt.Sprintf("HTTP_%d", resp.StatusCode)
				} else {
					_ = json.NewDecoder(resp.Body).Decode(&aiHealth)
				}
			}
			latestEvidence, evidenceGate, err := loadLatestEvidenceAndGate(c.Request.Context(), evidenceStore)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load readiness evidence"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
				"overallReady": dbReady && aiReachable,
				"db":           gin.H{"ready": dbReady, "error": dbErrOrNil(dbErr)},
				"ai":           gin.H{"aiReachable": aiReachable, "aiStatus": aiStatus, "aiHealth": aiHealth, "latencyMs": time.Since(start).Milliseconds()},
				"evidenceGate": evidenceGate, "latestEvidence": latestEvidence,
			})
		})

		v1.GET("/ops/dashboard", func(c *gin.Context) {
			if !assertAdmin(c, mustClaims(c)) {
				return
			}
			latestEvidence, evidenceGate, err := loadLatestEvidenceAndGate(c.Request.Context(), evidenceStore)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ops dashboard"})
				return
			}
			openIncidents, p0p1Incidents, err := opsSvc.DashboardCounts(c.Request.Context())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ops dashboard"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"latestEvidence": latestEvidence,
				"evidenceGate":   evidenceGate,
				"openIncidents":  openIncidents,
				"p0p1Incidents":  p0p1Incidents,
			})
		})

		v1.GET("/ops/evidence", func(c *gin.Context) {
			if !assertAdmin(c, mustClaims(c)) {
				return
			}
			rows, err := evidenceStore.ListEvidence(c.Request.Context(), parseLimit(c, 20))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list evidence"})
				return
			}
			items := make([]map[string]any, 0, len(rows))
			for _, row := range rows {
				items = append(items, evidenceRecordToMap(row))
			}
			c.JSON(http.StatusOK, items)
		})

		v1.POST("/ops/evidence", func(c *gin.Context) {
			cl := mustClaims(c)
			if !assertAdmin(c, cl) {
				return
			}
			var req struct {
				RunName                  string   `json:"runName"`
				ModelVersion             string   `json:"modelVersion"`
				DatasetName              string   `json:"datasetName"`
				DatasetVersion           string   `json:"datasetVersion"`
				SampleCount              int      `json:"sampleCount"`
				PositiveCount            int      `json:"positiveCount"`
				SiteCount                int      `json:"siteCount"`
				Auroc                    float64  `json:"auroc"`
				Sensitivity              float64  `json:"sensitivity"`
				Specificity              float64  `json:"specificity"`
				PPV                      *float64 `json:"ppv"`
				NPV                      *float64 `json:"npv"`
				ECE                      *float64 `json:"ece"`
				Brier                    *float64 `json:"brier"`
				CalibrationTemperature   *float64 `json:"calibrationTemperature"`
				ThresholdHighSensitivity *float64 `json:"thresholdHighSensitivity"`
				ThresholdHighSpecificity *float64 `json:"thresholdHighSpecificity"`
				StageRecommendation      string   `json:"stageRecommendation"`
				RegulatoryStatus         string   `json:"regulatoryStatus"`
				QAApprovedBy             string   `json:"qaApprovedBy"`
				MedicalApprovedBy        string   `json:"medicalApprovedBy"`
				ReportPath               string   `json:"reportPath"`
				Notes                    string   `json:"notes"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			if strings.TrimSpace(req.RunName) == "" || strings.TrimSpace(req.ModelVersion) == "" || strings.TrimSpace(req.DatasetName) == "" || req.SampleCount <= 0 || req.SiteCount <= 0 || req.PositiveCount < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid evidence payload"})
				return
			}
			if req.StageRecommendation == "" {
				req.StageRecommendation = "RESEARCH_ONLY"
			}
			if req.RegulatoryStatus == "" {
				req.RegulatoryStatus = "NOT_SUBMITTED"
			}
			row := &domain.ClinicalEvidenceRun{
				RunName:                  strings.TrimSpace(req.RunName),
				ModelVersion:             strings.TrimSpace(req.ModelVersion),
				DatasetName:              strings.TrimSpace(req.DatasetName),
				DatasetVersion:           toStrPtr(req.DatasetVersion),
				SampleCount:              req.SampleCount,
				PositiveCount:            req.PositiveCount,
				SiteCount:                req.SiteCount,
				Auroc:                    req.Auroc,
				Sensitivity:              req.Sensitivity,
				Specificity:              req.Specificity,
				PPV:                      req.PPV,
				NPV:                      req.NPV,
				ECE:                      req.ECE,
				Brier:                    req.Brier,
				CalibrationTemperature:   req.CalibrationTemperature,
				ThresholdHighSensitivity: req.ThresholdHighSensitivity,
				ThresholdHighSpecificity: req.ThresholdHighSpecificity,
				StageRecommendation:      req.StageRecommendation,
				RegulatoryStatus:         req.RegulatoryStatus,
				QAApprovedBy:             toStrPtr(req.QAApprovedBy),
				MedicalApprovedBy:        toStrPtr(req.MedicalApprovedBy),
				ReportPath:               toStrPtr(req.ReportPath),
				Notes:                    toStrPtr(req.Notes),
				CreatedByUserID:          &cl.UserID,
			}
			if err := opsSvc.CreateEvidence(c.Request.Context(), row); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create evidence"})
				return
			}
			c.JSON(http.StatusOK, evidenceRecordToMap(repository.EvidenceRecord{
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
			}))
		})

		v1.GET("/ops/incidents", func(c *gin.Context) {
			if !assertAdmin(c, mustClaims(c)) {
				return
			}
			rows, err := opsSvc.ListIncidents(c.Request.Context(), strings.TrimSpace(c.Query("status")), parseLimit(c, 30))
			if err != nil {
				if errors.Is(err, service.ErrInvalidIncidentStatus) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list incidents"})
				return
			}
			items := make([]map[string]any, 0, len(rows))
			for _, i := range rows {
				var owner any = nil
				if i.Owner != nil {
					owner = gin.H{"id": i.Owner.ID, "name": i.Owner.Name, "email": i.Owner.Email}
				}
				items = append(items, gin.H{
					"id":             i.ID,
					"source":         i.Source,
					"severity":       i.Severity,
					"status":         i.Status,
					"title":          i.Title,
					"detail":         strOrNil(i.Detail),
					"ownerUserId":    strOrNil(i.OwnerUserID),
					"openedAt":       i.OpenedAt,
					"acknowledgedAt": i.AcknowledgedAt,
					"resolvedAt":     i.ResolvedAt,
					"createdAt":      i.CreatedAt,
					"updatedAt":      i.UpdatedAt,
					"owner":          owner,
				})
			}
			c.JSON(http.StatusOK, items)
		})

		v1.POST("/ops/incidents", func(c *gin.Context) {
			if !assertAdmin(c, mustClaims(c)) {
				return
			}
			var req struct {
				Source      string `json:"source"`
				Severity    string `json:"severity"`
				Title       string `json:"title"`
				Detail      string `json:"detail"`
				OwnerUserID string `json:"ownerUserId"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			row, err := opsSvc.CreateIncident(c.Request.Context(), service.CreateIncidentInput{
				Source:      req.Source,
				Severity:    req.Severity,
				Title:       req.Title,
				Detail:      req.Detail,
				OwnerUserID: req.OwnerUserID,
			})
			if err != nil {
				if errors.Is(err, service.ErrInvalidIncidentPayload) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid incident payload"})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create incident"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"id":             row.ID,
				"source":         row.Source,
				"severity":       row.Severity,
				"status":         row.Status,
				"title":          row.Title,
				"detail":         strOrNil(row.Detail),
				"ownerUserId":    strOrNil(row.OwnerUserID),
				"openedAt":       row.OpenedAt,
				"acknowledgedAt": row.AcknowledgedAt,
				"resolvedAt":     row.ResolvedAt,
				"createdAt":      row.CreatedAt,
				"updatedAt":      row.UpdatedAt,
			})
		})

		v1.POST("/ops/incidents/:id/transition", func(c *gin.Context) {
			cl := mustClaims(c)
			if !assertAdmin(c, cl) {
				return
			}
			var req struct {
				Action string `json:"action"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			row, _, err := opsSvc.TransitionIncident(c.Request.Context(), c.Param("id"), req.Action, cl.UserID)
			if err != nil {
				switch {
				case errors.Is(err, service.ErrInvalidIncidentTransition):
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid incident transition"})
				case errors.Is(err, gorm.ErrRecordNotFound):
					c.JSON(http.StatusNotFound, gin.H{"error": "Incident not found"})
				default:
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transition incident"})
				}
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"id":             row.ID,
				"source":         row.Source,
				"severity":       row.Severity,
				"status":         row.Status,
				"title":          row.Title,
				"detail":         strOrNil(row.Detail),
				"ownerUserId":    strOrNil(row.OwnerUserID),
				"openedAt":       row.OpenedAt,
				"acknowledgedAt": row.AcknowledgedAt,
				"resolvedAt":     row.ResolvedAt,
				"createdAt":      row.CreatedAt,
				"updatedAt":      row.UpdatedAt,
			})
		})
	}

	grpcPort := strings.TrimSpace(os.Getenv("GRPC_PORT"))
	if grpcPort == "" {
		grpcPort = "9083"
	}
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("governance grpc listen error: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodec(rpccodec.New()),
		grpc.UnaryInterceptor(observability.UnaryServerTraceInterceptor("governance-service.grpc-server")),
	)
	bridge.RegisterServiceServer(grpcServer, bridgehttp.New(r))
	httpServer := &http.Server{Addr: ":" + port, Handler: r}
	if err := lifecycle.Run(context.Background(), lifecycle.Runtime{
		Name:            "governance-service",
		HTTPServer:      httpServer,
		GRPCServer:      grpcServer,
		GRPCListener:    lis,
		ShutdownTimeout: 10 * time.Second,
	}); err != nil {
		log.Fatal(err)
	}
}

func authRequired(secret []byte) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing bearer token"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
		cl := &claims{}
		parsed, err := jwt.ParseWithClaims(token, cl, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return secret, nil
		})
		if err != nil || !parsed.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			return
		}
		c.Set("claims", cl)
		c.Next()
	}
}

func mustClaims(c *gin.Context) *claims {
	raw, _ := c.Get("claims")
	out, _ := raw.(*claims)
	return out
}

func assertAdmin(c *gin.Context, cl *claims) bool {
	if cl == nil || cl.Role != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can access this endpoint"})
		return false
	}
	return true
}

func parseLimit(c *gin.Context, fallback int) int {
	v := strings.TrimSpace(c.Query("limit"))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 100 {
		return 100
	}
	return n
}

func strOrNil(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func dbErrOrNil(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func toStrPtr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func loadLatestEvidenceAndGate(ctx context.Context, store repository.EvidenceStore) (map[string]any, map[string]any, error) {
	rec, err := store.GetLatestEvidence(ctx)
	if err != nil {
		return nil, nil, err
	}
	if rec == nil {
		return nil, nil, nil
	}
	latest := evidenceRecordToMap(*rec)
	return latest, evaluateEvidenceGate(latest), nil
}

func evidenceRecordToMap(rec repository.EvidenceRecord) map[string]any {
	var createdBy any = nil
	if rec.CreatedBy != nil {
		createdBy = gin.H{"id": rec.CreatedBy.ID, "name": rec.CreatedBy.Name, "email": rec.CreatedBy.Email}
	}
	return map[string]any{
		"id":                       rec.ID,
		"runName":                  rec.RunName,
		"modelVersion":             rec.ModelVersion,
		"datasetName":              rec.DatasetName,
		"datasetVersion":           strOrNil(rec.DatasetVersion),
		"sampleCount":              rec.SampleCount,
		"positiveCount":            rec.PositiveCount,
		"siteCount":                rec.SiteCount,
		"auroc":                    rec.Auroc,
		"sensitivity":              rec.Sensitivity,
		"specificity":              rec.Specificity,
		"ppv":                      rec.PPV,
		"npv":                      rec.NPV,
		"ece":                      rec.ECE,
		"brier":                    rec.Brier,
		"calibrationTemperature":   rec.CalibrationTemperature,
		"thresholdHighSensitivity": rec.ThresholdHighSensitivity,
		"thresholdHighSpecificity": rec.ThresholdHighSpecificity,
		"regulatoryStatus":         rec.RegulatoryStatus,
		"stageRecommendation":      rec.StageRecommendation,
		"qaApprovedBy":             strOrNil(rec.QAApprovedBy),
		"medicalApprovedBy":        strOrNil(rec.MedicalApprovedBy),
		"reportPath":               strOrNil(rec.ReportPath),
		"notes":                    strOrNil(rec.Notes),
		"createdByUserId":          strOrNil(rec.CreatedByUserID),
		"createdAt":                rec.CreatedAt,
		"updatedAt":                rec.UpdatedAt,
		"createdBy":                createdBy,
	}
}

func evaluateEvidenceGate(evidence map[string]any) map[string]any {
	minSiteCount := parseIntEnv("AI_GOV_MIN_SITE_COUNT", 2)
	minAuroc := parseFloatEnv("AI_GOV_MIN_AUROC", 0.90)
	minSensitivity := parseFloatEnv("AI_GOV_MIN_SENSITIVITY", 0.90)
	minSpecificity := parseFloatEnv("AI_GOV_MIN_SPECIFICITY", 0.85)
	siteCount := intOrZero(anyToIntPtr(evidence["siteCount"]))
	auroc := floatOrZero(anyToFloatPtr(evidence["auroc"]))
	sensitivity := floatOrZero(anyToFloatPtr(evidence["sensitivity"]))
	specificity := floatOrZero(anyToFloatPtr(evidence["specificity"]))
	regulatoryStatus, _ := evidence["regulatoryStatus"].(string)
	pass := siteCount >= minSiteCount && auroc >= minAuroc && sensitivity >= minSensitivity && specificity >= minSpecificity && regulatoryStatus == "APPROVED"
	return map[string]any{
		"pass": pass,
		"criteria": map[string]any{
			"minSiteCount":   minSiteCount,
			"minAuroc":       minAuroc,
			"minSensitivity": minSensitivity,
			"minSpecificity": minSpecificity,
		},
	}
}

func parseFloatEnv(name string, fallback float64) float64 {
	value := strings.TrimSpace(envOr(name, ""))
	if value == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return n
}

func parseIntEnv(name string, fallback int) int {
	value := strings.TrimSpace(envOr(name, ""))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func floatOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func anyToFloatPtr(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		x := float64(n)
		return &x
	case int:
		x := float64(n)
		return &x
	case int64:
		x := float64(n)
		return &x
	default:
		return nil
	}
}

func anyToIntPtr(v any) *int {
	switch n := v.(type) {
	case int:
		return &n
	case int64:
		x := int(n)
		return &x
	case float64:
		x := int(n)
		return &x
	default:
		return nil
	}
}
