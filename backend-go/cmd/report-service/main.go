package main

// File: cmd/report-service/main.go
// Purpose: Report microservice entrypoint and report API handlers.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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
	traceShutdown, err := observability.InitTracingFromEnv(context.Background(), "report-service")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = traceShutdown(context.Background())
	}()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8082"
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := strings.TrimSpace(os.Getenv("BACKEND_JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}
	gdb, err := gormdb.Open(dsn)
	if err != nil {
		log.Fatalf("init gorm: %v", err)
	}
	reportSvc := service.NewReportService(repository.NewGormReportRepository(gdb))

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "report-service"})
	})

	v1 := r.Group("/api/v1")
	v1.Use(authRequired([]byte(jwtSecret)))
	{
		v1.GET("/reports", func(c *gin.Context) {
			cl := mustClaims(c)
			status := strings.TrimSpace(c.Query("status"))
			patientID := strings.TrimSpace(c.Query("patientId"))
			limit := parseLimit(c, 20)

			patientUserID := ""
			if cl.Role == "PATIENT" {
				patientUserID = cl.UserID
			}
			rows, next, err := reportSvc.List(c.Request.Context(), service.ListReportsInput{
				Status:        status,
				PatientID:     patientID,
				PatientUserID: patientUserID,
				Limit:         limit,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list reports"})
				return
			}
			items := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				items = append(items, gin.H{
					"id":          r.ID,
					"detectionId": r.DetectionID,
					"patientId":   r.PatientID,
					"doctorId":    r.DoctorID,
					"content":     toIfaceMap(r.Content),
					"pdfPath":     strOrNil(r.PDFPath),
					"status":      r.Status,
					"createdAt":   r.CreatedAt,
					"updatedAt":   r.UpdatedAt,
				})
			}
			var nextCursor any = nil
			if next != nil {
				nextCursor = *next
			}
			c.JSON(http.StatusOK, gin.H{"reports": items, "nextCursor": nextCursor})
		})

		v1.GET("/reports/:id", func(c *gin.Context) {
			cl := mustClaims(c)
			row, err := reportSvc.GetByID(c.Request.Context(), c.Param("id"))
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
				return
			}
			if cl.Role == "PATIENT" && row.Patient.UserID != cl.UserID {
				c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"id":          row.ID,
				"detectionId": row.DetectionID,
				"patientId":   row.PatientID,
				"doctorId":    row.DoctorID,
				"content":     toIfaceMap(row.Content),
				"pdfPath":     strOrNil(row.PDFPath),
				"status":      row.Status,
				"createdAt":   row.CreatedAt,
				"updatedAt":   row.UpdatedAt,
			})
		})

		v1.POST("/reports", func(c *gin.Context) {
			cl := mustClaims(c)
			if cl.Role != "DOCTOR" && cl.Role != "ADMIN" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can create reports"})
				return
			}
			var req struct {
				DetectionID string `json:"detectionId"`
				PatientID   string `json:"patientId"`
				Content     any    `json:"content"`
				Status      string `json:"status"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			row, _, err := reportSvc.Upsert(c.Request.Context(), service.UpsertReportInput{
				DetectionID: req.DetectionID,
				PatientID:   req.PatientID,
				DoctorID:    cl.UserID,
				Content:     req.Content,
				Status:      req.Status,
			})
			if err != nil {
				switch {
				case errors.Is(err, service.ErrInvalidReportPayload), errors.Is(err, service.ErrInvalidReportStatus), errors.Is(err, service.ErrDetectionNotReviewed), errors.Is(err, service.ErrPatientMismatch):
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				case errors.Is(err, gorm.ErrRecordNotFound):
					c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
				default:
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create report"})
				}
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"id":          row.ID,
				"detectionId": row.DetectionID,
				"patientId":   row.PatientID,
				"doctorId":    row.DoctorID,
				"content":     toIfaceMap(row.Content),
				"pdfPath":     strOrNil(row.PDFPath),
				"status":      row.Status,
				"createdAt":   row.CreatedAt,
				"updatedAt":   row.UpdatedAt,
			})
		})

		v1.PATCH("/reports/:id", func(c *gin.Context) {
			cl := mustClaims(c)
			if cl.Role != "DOCTOR" && cl.Role != "ADMIN" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can update reports"})
				return
			}
			var req struct {
				Content *any   `json:"content"`
				Status  string `json:"status"`
				PDFPath string `json:"pdfPath"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			row, _, _, err := reportSvc.Update(c.Request.Context(), service.UpdateReportInput{
				ID:      c.Param("id"),
				Content: req.Content,
				Status:  req.Status,
				PDFPath: req.PDFPath,
			})
			if err != nil {
				switch {
				case errors.Is(err, service.ErrInvalidReportStatus), errors.Is(err, service.ErrInvalidReportTransition), errors.Is(err, service.ErrNoReportFieldsToUpdate):
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				case errors.Is(err, gorm.ErrRecordNotFound):
					c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
				default:
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update report"})
				}
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"id":          row.ID,
				"detectionId": row.DetectionID,
				"patientId":   row.PatientID,
				"doctorId":    row.DoctorID,
				"content":     toIfaceMap(row.Content),
				"pdfPath":     strOrNil(row.PDFPath),
				"status":      row.Status,
				"createdAt":   row.CreatedAt,
				"updatedAt":   row.UpdatedAt,
			})
		})
	}

	grpcPort := strings.TrimSpace(os.Getenv("GRPC_PORT"))
	if grpcPort == "" {
		grpcPort = "9082"
	}
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("report grpc listen error: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodec(rpccodec.New()),
		grpc.UnaryInterceptor(observability.UnaryServerTraceInterceptor("report-service.grpc-server")),
	)
	bridge.RegisterServiceServer(grpcServer, bridgehttp.New(r))
	httpServer := &http.Server{Addr: ":" + port, Handler: r}
	if err := lifecycle.Run(context.Background(), lifecycle.Runtime{
		Name:            "report-service",
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

func toIfaceMap(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}
