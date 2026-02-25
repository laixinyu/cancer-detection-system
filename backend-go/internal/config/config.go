package config

// File: internal/config/config.go
// Purpose: Configuration parsing and normalization from environment variables.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                  string
	DatabaseURL           string
	JWTSecret             string
	AIServiceURL          string
	DetectionServiceURL   string
	ReportServiceURL      string
	GovernanceServiceURL  string
	DetectionServiceGRPC  string
	ReportServiceGRPC     string
	GovernanceServiceGRPC string
	CacheBackend          string
	UploadDir             string
	UploadPublicPrefix    string
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	ShutdownTimeout       time.Duration
	RequestTimeout        time.Duration
	LogLevel              string
	SlowRequestThreshold  time.Duration
	InboundRateLimitRPS   float64
	InboundRateLimitBurst int
	AIOutboundRPS         float64
	AIOutboundBurst       int
	BreakerFailThreshold  int
	BreakerOpenTimeout    time.Duration
	BreakerHalfOpenCalls  int
	TraceEnabled          bool
	TraceServiceName      string
	TraceEndpoint         string
	TraceSampleRatio      float64
}

func Load() (*Config, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	jwtSecret := strings.TrimSpace(os.Getenv("BACKEND_JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}

	cfg := &Config{
		Port:                  envOr("PORT", "8080"),
		DatabaseURL:           dsn,
		JWTSecret:             jwtSecret,
		AIServiceURL:          strings.TrimRight(envOr("AI_SERVICE_URL", "http://localhost:8000"), "/"),
		DetectionServiceURL:   normalizeURL(envOr("DETECTION_SERVICE_URL", "")),
		ReportServiceURL:      normalizeURL(envOr("REPORT_SERVICE_URL", "")),
		GovernanceServiceURL:  normalizeURL(envOr("GOVERNANCE_SERVICE_URL", "")),
		DetectionServiceGRPC:  strings.TrimSpace(envOr("DETECTION_SERVICE_GRPC_ADDR", "")),
		ReportServiceGRPC:     strings.TrimSpace(envOr("REPORT_SERVICE_GRPC_ADDR", "")),
		GovernanceServiceGRPC: strings.TrimSpace(envOr("GOVERNANCE_SERVICE_GRPC_ADDR", "")),
		CacheBackend:          strings.ToLower(envOr("CACHE_BACKEND", "memory")),
		UploadDir:             envOr("UPLOAD_DIR", "public/uploads"),
		UploadPublicPrefix:    strings.TrimRight(envOr("UPLOAD_PUBLIC_PREFIX", "/uploads"), "/"),
		ReadTimeout:           15 * time.Second,
		WriteTimeout:          30 * time.Second,
		IdleTimeout:           60 * time.Second,
		ShutdownTimeout:       10 * time.Second,
		RequestTimeout:        30 * time.Second,
		LogLevel:              strings.ToLower(envOr("LOG_LEVEL", "info")),
		SlowRequestThreshold:  time.Duration(parseIntEnv("ALERT_SLOW_REQUEST_MS", 2000)) * time.Millisecond,
		InboundRateLimitRPS:   parseFloatEnv("INBOUND_RATE_LIMIT_RPS", 30),
		InboundRateLimitBurst: parseIntEnv("INBOUND_RATE_LIMIT_BURST", 60),
		AIOutboundRPS:         parseFloatEnv("AI_OUTBOUND_RATE_LIMIT_RPS", 8),
		AIOutboundBurst:       parseIntEnv("AI_OUTBOUND_RATE_LIMIT_BURST", 16),
		BreakerFailThreshold:  parseIntEnv("UPSTREAM_BREAKER_FAIL_THRESHOLD", 5),
		BreakerOpenTimeout:    time.Duration(parseIntEnv("UPSTREAM_BREAKER_OPEN_SECONDS", 15)) * time.Second,
		BreakerHalfOpenCalls:  parseIntEnv("UPSTREAM_BREAKER_HALF_OPEN_CALLS", 2),
		TraceEnabled:          parseBoolEnv("OTEL_ENABLED", false),
		TraceServiceName:      envOr("OTEL_SERVICE_NAME", "cancer-detection-gateway"),
		TraceEndpoint:         strings.TrimSpace(envOr("OTEL_EXPORTER_OTLP_ENDPOINT", "")),
		TraceSampleRatio:      parseFloatEnv("OTEL_TRACE_SAMPLE_RATIO", 1.0),
	}
	return cfg, nil
}

func (c *Config) Addr() string {
	return ":" + c.Port
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return strings.TrimRight(raw, "/")
}

func parseIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func parseFloatEnv(key string, fallback float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func parseBoolEnv(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
