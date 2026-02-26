package config

// 文件： internal/config/config.go
// 用途：配置加载，从环境变量解析并规范化配置。

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                string
	MicroserviceMode      string
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
	CacheRequired         bool
	RedisAddr             string
	RedisPassword         string
	RedisDB               int
	GRPCInsecure          bool
	GRPCServerName        string
	GRPCCACertFile        string
	EventBusBackend       string
	EventBusRequired      bool
	EventBusRedisAddr     string
	EventBusRedisPassword string
	EventBusRedisDB       int
	EventBusRedisStream   string
	OutboxRelayEnabled    bool
	OutboxRelayBatch      int
	OutboxMaxAttempts     int
	OutboxRelayInterval   time.Duration
	IdempotencyEnabled    bool
	IdempotencyRequired   bool
	IdempotencyTTL        time.Duration
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

	appEnv := strings.ToLower(strings.TrimSpace(envOr("APP_ENV", "development")))
	cacheBackend := strings.ToLower(strings.TrimSpace(os.Getenv("CACHE_BACKEND")))
	isProd := appEnv == "production" || appEnv == "prod"
	microserviceMode := strings.ToLower(strings.TrimSpace(envOr("MICROSERVICE_MODE", "")))
	if microserviceMode == "" {
		if isProd {
			microserviceMode = "strict"
		} else {
			microserviceMode = "compat"
		}
	}
	if microserviceMode != "strict" && microserviceMode != "compat" {
		return nil, fmt.Errorf("MICROSERVICE_MODE must be strict or compat")
	}

	if cacheBackend == "" {
		if isProd {
			cacheBackend = "redis"
		} else {
			cacheBackend = "memory"
		}
	}

	cfg := &Config{
		AppEnv:                appEnv,
		MicroserviceMode:      microserviceMode,
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
		CacheBackend:          cacheBackend,
		CacheRequired:         parseBoolEnv("CACHE_REQUIRED", false),
		RedisAddr:             envOr("REDIS_ADDR", "localhost:6379"),
		RedisPassword:         envOr("REDIS_PASSWORD", ""),
		RedisDB:               parseNonNegativeIntEnv("REDIS_DB", 0),
		GRPCInsecure:          parseBoolEnv("GRPC_INSECURE", !isProd),
		GRPCServerName:        strings.TrimSpace(envOr("GRPC_SERVER_NAME", "")),
		GRPCCACertFile:        strings.TrimSpace(envOr("GRPC_CA_CERT_FILE", "")),
		EventBusBackend:       strings.ToLower(strings.TrimSpace(envOr("EVENT_BUS_BACKEND", "log"))),
		EventBusRequired:      parseBoolEnv("EVENT_BUS_REQUIRED", false),
		EventBusRedisAddr:     envOr("EVENT_BUS_REDIS_ADDR", "localhost:6379"),
		EventBusRedisPassword: envOr("EVENT_BUS_REDIS_PASSWORD", ""),
		EventBusRedisDB:       parseNonNegativeIntEnv("EVENT_BUS_REDIS_DB", 0),
		EventBusRedisStream:   envOr("EVENT_BUS_REDIS_STREAM", "domain_events"),
		OutboxRelayEnabled:    parseBoolEnv("OUTBOX_RELAY_ENABLED", true),
		OutboxRelayBatch:      parseIntEnv("OUTBOX_RELAY_BATCH", 50),
		OutboxMaxAttempts:     parseIntEnv("OUTBOX_MAX_ATTEMPTS", 8),
		OutboxRelayInterval:   time.Duration(parseIntEnv("OUTBOX_RELAY_INTERVAL_MS", 1000)) * time.Millisecond,
		IdempotencyEnabled:    parseBoolEnv("IDEMPOTENCY_ENABLED", true),
		IdempotencyRequired:   parseBoolEnv("IDEMPOTENCY_REQUIRED", true),
		IdempotencyTTL:        time.Duration(parseIntEnv("IDEMPOTENCY_TTL_SECONDS", 86400)) * time.Second,
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

func (c *Config) StrictMicroserviceMode() bool {
	return strings.EqualFold(c.MicroserviceMode, "strict")
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

func parseNonNegativeIntEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
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
