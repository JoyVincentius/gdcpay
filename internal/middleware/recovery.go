package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Recovery is the global panic handler. It converts any panic raised inside
// a handler into the API's structured 500 response instead of crashing the
// process or leaking a stack trace to the client.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered",
					"request_id", requestID(c),
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"error", rec,
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"status":    http.StatusInternalServerError,
					"code":      "INTERNAL_ERROR",
					"message":   "internal server error",
					"timestamp": time.Now().UTC(),
				})
			}
		}()
		c.Next()
	}
}
