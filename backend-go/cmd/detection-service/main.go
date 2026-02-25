package main

// 文件： cmd/detection-service/main.go
// 用途：检测微服务入口与检测 API 处理逻辑。

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
	traceShutdown, err := observability.InitTracingFromEnv(context.Background(), "detection-service")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = traceShutdown(context.Background())
	}()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	jwtSecret := strings.TrimSpace(os.Getenv("BACKEND_JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}

	gdb, err := repositoryOpen(dsn)
	if err != nil {
		log.Fatalf("init gorm: %v", err)
	}
	detectionSvc := service.NewDetectionService(repository.NewGormDetectionRepository(gdb))

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "detection-service"})
	})

	v1 := r.Group("/api/v1")
	v1.Use(authRequired([]byte(jwtSecret)))
	{
		v1.GET("/detections", func(c *gin.Context) {
			cl := mustClaims(c)
			status := strings.TrimSpace(c.Query("status"))
			priority := strings.TrimSpace(c.Query("priority"))
			orderByPriority := strings.EqualFold(c.DefaultQuery("orderByPriority", "true"), "true")
			limit := parseLimit(c, 20)

			patientUserID := ""
			if cl.Role == "PATIENT" {
				patientUserID = cl.UserID
			}
			rows, next, err := detectionSvc.List(c.Request.Context(), service.ListDetectionsInput{
				Status:          status,
				Priority:        priority,
				OrderByPriority: orderByPriority,
				PatientUserID:   patientUserID,
				Limit:           limit,
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list detections"})
				return
			}

			items := make([]map[string]any, 0, len(rows))
			for _, d := range rows {
				var reviewer any = nil
				if d.Reviewer != nil {
					reviewer = gin.H{"name": d.Reviewer.Name, "email": d.Reviewer.Email}
				}
				items = append(items, gin.H{
					"id":                d.ID,
					"imageId":           d.ImageID,
					"modelVersion":      d.ModelVersion,
					"cancerProbability": d.CancerProbability,
					"findings":          toIfaceMap(d.Findings),
					"heatmapPath":       strOrNil(d.HeatmapPath),
					"status":            d.Status,
					"reviewedBy":        strOrNil(d.ReviewedBy),
					"reviewNotes":       strOrNil(d.ReviewNotes),
					"createdAt":         d.CreatedAt,
					"updatedAt":         d.UpdatedAt,
					"image": gin.H{
						"id":           d.Image.ID,
						"patientId":    d.Image.PatientID,
						"fileType":     d.Image.FileType,
						"originalName": d.Image.OriginalName,
						"fileSize":     d.Image.FileSize,
						"status":       d.Image.Status,
						"uploadedBy":   d.Image.UploadedBy,
						"createdAt":    d.Image.CreatedAt,
						"updatedAt":    d.Image.UpdatedAt,
						"patient":      gin.H{"user": gin.H{"name": d.Image.Patient.User.Name, "email": d.Image.Patient.User.Email}},
					},
					"reviewer": reviewer,
				})
			}
			var nextCursor any = nil
			if next != nil {
				nextCursor = *next
			}
			c.JSON(http.StatusOK, gin.H{"detections": items, "nextCursor": nextCursor})
		})

		v1.GET("/detections/:id", func(c *gin.Context) {
			cl := mustClaims(c)
			id := c.Param("id")
			d, err := detectionSvc.GetByID(c.Request.Context(), id)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
				return
			}
			if cl.Role == "PATIENT" && d.Image.UploadedBy != cl.UserID {
				c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
				return
			}
			var reviewer any = nil
			if d.Reviewer != nil {
				reviewer = gin.H{"name": d.Reviewer.Name, "email": d.Reviewer.Email}
			}
			c.JSON(http.StatusOK, gin.H{
				"id":                d.ID,
				"imageId":           d.ImageID,
				"modelVersion":      d.ModelVersion,
				"cancerProbability": d.CancerProbability,
				"findings":          toIfaceMap(d.Findings),
				"heatmapPath":       strOrNil(d.HeatmapPath),
				"status":            d.Status,
				"reviewedBy":        strOrNil(d.ReviewedBy),
				"reviewNotes":       strOrNil(d.ReviewNotes),
				"createdAt":         d.CreatedAt,
				"updatedAt":         d.UpdatedAt,
				"reviewer":          reviewer,
			})
		})

		v1.POST("/detections/:id/review", func(c *gin.Context) {
			cl := mustClaims(c)
			if cl.Role != "DOCTOR" && cl.Role != "ADMIN" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can review detections"})
				return
			}
			id := c.Param("id")
			var req struct {
				Status      string `json:"status"`
				ReviewNotes string `json:"reviewNotes"`
				Findings    any    `json:"findings"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			row, _, err := detectionSvc.Review(c.Request.Context(), service.ReviewDetectionInput{
				ID:          id,
				Status:      req.Status,
				ReviewerID:  cl.UserID,
				ReviewNotes: req.ReviewNotes,
				Findings:    req.Findings,
			})
			if err != nil {
				switch {
				case errors.Is(err, service.ErrInvalidDetectionStatus), errors.Is(err, service.ErrInvalidDetectionTransition):
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				case errors.Is(err, gorm.ErrRecordNotFound):
					c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
				default:
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to review detection"})
				}
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"id":                row.ID,
				"imageId":           row.ImageID,
				"modelVersion":      row.ModelVersion,
				"cancerProbability": row.CancerProbability,
				"findings":          toIfaceMap(row.Findings),
				"heatmapPath":       strOrNil(row.HeatmapPath),
				"status":            row.Status,
				"reviewedBy":        strOrNil(row.ReviewedBy),
				"reviewNotes":       strOrNil(row.ReviewNotes),
				"createdAt":         row.CreatedAt,
				"updatedAt":         row.UpdatedAt,
			})
		})
	}

	grpcPort := strings.TrimSpace(os.Getenv("GRPC_PORT"))
	if grpcPort == "" {
		grpcPort = "9081"
	}
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("detection grpc listen error: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodec(rpccodec.New()),
		grpc.UnaryInterceptor(observability.UnaryServerTraceInterceptor("detection-service.grpc-server")),
	)
	bridge.RegisterServiceServer(grpcServer, bridgehttp.New(r))

	httpServer := &http.Server{Addr: ":" + port, Handler: r}
	if err := lifecycle.Run(context.Background(), lifecycle.Runtime{
		Name:            "detection-service",
		HTTPServer:      httpServer,
		GRPCServer:      grpcServer,
		GRPCListener:    lis,
		ShutdownTimeout: 10 * time.Second,
	}); err != nil {
		log.Fatal(err)
	}
}

func repositoryOpen(dsn string) (*gorm.DB, error) {
	return gormdb.Open(dsn)
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
