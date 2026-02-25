package bridgehttp

// File: internal/rpc/bridgehttp/handler.go
// Purpose: Internal gRPC bridge contracts, codec, and HTTP bridge adapter.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"cancer-detection-backend/internal/rpc/bridge"
)

type Handler struct {
	router http.Handler
}

func New(router http.Handler) *Handler {
	return &Handler{router: router}
}

func (h *Handler) Handle(_ context.Context, req *bridge.RequestEnvelope) (*bridge.ResponseEnvelope, error) {
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
	httpReq, err := http.NewRequest(req.Method, path, bodyReader)
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
