package main

// File: cmd/server/grpc_bridge_handler.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

import (
	"encoding/json"
	"net/http"

	"cancer-detection-backend/internal/rpc/bridge"

	"github.com/gin-gonic/gin"
)

func (a *app) grpcBridgeHandler(client bridge.ServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body any
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodDelete {
			if c.Request.ContentLength > 0 {
				if err := c.ShouldBindJSON(&body); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
					return
				}
			}
		}

		query := map[string]string{}
		for k, v := range c.Request.URL.Query() {
			if len(v) > 0 {
				query[k] = v[0]
			}
		}
		headers := map[string]string{
			"Authorization": c.GetHeader("Authorization"),
			"Content-Type":  c.GetHeader("Content-Type"),
		}
		req := &bridge.RequestEnvelope{
			Method:  c.Request.Method,
			Path:    c.Request.URL.Path,
			Query:   query,
			Headers: headers,
			Body:    body,
		}
		resp, err := client.Handle(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream gRPC unavailable"})
			return
		}

		status := resp.Status
		if status == 0 {
			status = http.StatusOK
		}
		if resp.Body == nil {
			c.Status(status)
			return
		}
		switch b := resp.Body.(type) {
		case map[string]any, []any:
			c.JSON(status, b)
			return
		default:
			// try to preserve JSON body from generic decode
			raw, _ := json.Marshal(b)
			var obj any
			if err := json.Unmarshal(raw, &obj); err == nil {
				c.JSON(status, obj)
				return
			}
			c.JSON(status, gin.H{"data": b})
		}
	}
}
