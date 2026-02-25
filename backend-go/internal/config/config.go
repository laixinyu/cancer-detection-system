package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                 string
	DatabaseURL          string
	JWTSecret            string
	AIServiceURL         string
	DetectionServiceURL  string
	ReportServiceURL     string
	GovernanceServiceURL string
	CacheBackend         string
	UploadDir            string
	UploadPublicPrefix   string
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	ShutdownTimeout      time.Duration
	RequestTimeout       time.Duration
	LogLevel             string
	SlowRequestThreshold time.Duration
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
		Port:                 envOr("PORT", "8080"),
		DatabaseURL:          dsn,
		JWTSecret:            jwtSecret,
		AIServiceURL:         strings.TrimRight(envOr("AI_SERVICE_URL", "http://localhost:8000"), "/"),
		DetectionServiceURL:  normalizeURL(envOr("DETECTION_SERVICE_URL", "")),
		ReportServiceURL:     normalizeURL(envOr("REPORT_SERVICE_URL", "")),
		GovernanceServiceURL: normalizeURL(envOr("GOVERNANCE_SERVICE_URL", "")),
		CacheBackend:         strings.ToLower(envOr("CACHE_BACKEND", "memory")),
		UploadDir:            envOr("UPLOAD_DIR", "public/uploads"),
		UploadPublicPrefix:   strings.TrimRight(envOr("UPLOAD_PUBLIC_PREFIX", "/uploads"), "/"),
		ReadTimeout:          15 * time.Second,
		WriteTimeout:         30 * time.Second,
		IdleTimeout:          60 * time.Second,
		ShutdownTimeout:      10 * time.Second,
		RequestTimeout:       30 * time.Second,
		LogLevel:             strings.ToLower(envOr("LOG_LEVEL", "info")),
		SlowRequestThreshold: time.Duration(parseIntEnv("ALERT_SLOW_REQUEST_MS", 2000)) * time.Millisecond,
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
