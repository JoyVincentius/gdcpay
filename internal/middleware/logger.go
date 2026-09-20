package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "request_id"

// RequestLogger assigns a request_id to every request and emits one
// structured JSON log line per request with method, path, status code and
// latency. Level is INFO for 2xx/3xx, WARN for 4xx, ERROR for 5xx.
func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set(RequestIDKey, reqID)
		c.Writer.Header().Set("X-Request-ID", reqID)

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"request_id", reqID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status_code", status,
			"latency_ms", time.Since(start).Milliseconds(),
		}

		switch {
		case status >= 500:
			logger.Error("request completed", attrs...)
		case status >= 400:
			logger.Warn("request completed", attrs...)
		default:
			logger.Info("request completed", attrs...)
		}
	}
}

func requestID(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}
