package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const CorrelationIDHeader = "X-Correlation-ID"

func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(CorrelationIDHeader)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("correlation_id", id)
		c.Header(CorrelationIDHeader, id)

		start := time.Now()
		c.Next()

		slog.Info("request completed",
			"correlation_id", id,
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}
