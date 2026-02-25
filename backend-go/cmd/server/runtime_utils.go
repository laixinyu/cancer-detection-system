package main

// File: cmd/server/runtime_utils.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

import (
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func getClaims(c *gin.Context) *authClaims {
	raw, ok := c.Get("claims")
	if !ok {
		return nil
	}
	claims, ok := raw.(*authClaims)
	if !ok {
		return nil
	}
	return claims
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func nullIfEmpty(v string) interface{} {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func dbErrOrNil(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func isAllowedExt(ext string) bool {
	switch ext {
	case "PNG", "JPEG", "TIFF", "DCM":
		return true
	default:
		return false
	}
}

func fileTypeEnum(ext string) string {
	switch ext {
	case "PNG":
		return "PNG"
	case "TIFF":
		return "TIFF"
	case "DCM":
		return "DICOM"
	default:
		return "JPEG"
	}
}

func normalizeExt(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".png", ".jpg", ".jpeg", ".tif", ".tiff", ".dcm":
		return ext
	default:
		return ".jpg"
	}
}

func boolValue(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func mimeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".dcm":
		return "application/dicom"
	default:
		return "application/octet-stream"
	}
}

func urlEscapeFilename(name string) string {
	replacer := strings.NewReplacer("\n", "", "\r", "", "\"", "")
	return replacer.Replace(name)
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
