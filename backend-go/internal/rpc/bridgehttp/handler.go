package bridgehttp

// 文件： internal/rpc/bridgehttp/handler.go
// 用途：内部 gRPC 桥接协议、编解码器与 HTTP 适配器。

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/rpc/bridge"
)

type Handler struct {
	router http.Handler
}

func New(router http.Handler) *Handler {
	return &Handler{router: router}
}

func (h *Handler) Handle(ctx context.Context, req *bridge.RequestEnvelope) (*bridge.ResponseEnvelope, error) {
	path := req.Path
	values := url.Values{}
	for k, v := range req.Query {
		values.Set(k, v)
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var bodyReader *bytes.Reader
	if req.Body != nil {
		raw, _ := json.Marshal(req.Body)
		bodyReader = bytes.NewReader(raw)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, path, bodyReader)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		if strings.TrimSpace(v) != "" {
			httpReq.Header.Set(k, v)
		}
	}
	if req.Body != nil && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if requestID := strings.TrimSpace(httpReq.Header.Get("X-Request-Id")); requestID == "" {
		if requestID = observability.RequestIDFromContext(ctx); requestID != "" {
			httpReq.Header.Set("X-Request-Id", requestID)
		}
	}

	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, httpReq)

	respBody := rec.Body.Bytes()
	if len(respBody) == 0 {
		return &bridge.ResponseEnvelope{Status: rec.Code, Body: nil}, nil
	}
	var parsed any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		parsed = map[string]any{"raw": string(respBody)}
	}
	return &bridge.ResponseEnvelope{Status: rec.Code, Body: parsed}, nil
}
