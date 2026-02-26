package main

import (
	"os"
	"strconv"
	"strings"
)

func parseLimit(raw string, fallback int) int {
	v := strings.TrimSpace(raw)
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
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func parseBoolEnv(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(envOr(name, "")))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
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
