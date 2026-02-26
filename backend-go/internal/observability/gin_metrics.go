package observability

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func GinMetricsMiddleware(reg *Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if reg == nil {
			return
		}
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		reg.ObserveHTTP(c.Request.Method, route, c.Writer.Status(), time.Since(start).Milliseconds())
	}
}

func GinMetricsHandler(reg *Registry) gin.HandlerFunc {
	return func(c *gin.Context) {
		if reg == nil {
			c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(""))
			return
		}
		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(reg.PrometheusText()))
	}
}
