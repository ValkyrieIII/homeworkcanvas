package shared

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDKey = "request_id"

func ConfigureLogging() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
}

// Gin's default recovery can dump custom request headers in debug mode.
func SafeRecovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, recovered any) {
		slog.Error("http_panic", "request_id", RequestID(c),
			"panic_type", fmt.Sprintf("%T", recovered), "stack", string(debug.Stack()))
		c.AbortWithStatusJSON(http.StatusInternalServerError,
			gin.H{"error": gin.H{"code": "shared.internal", "message": "服务暂时不可用"}})
	})
}

// RequestLog records route templates only: URLs, query strings and bodies may contain course data.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.NewString()
		started := time.Now()
		c.Set(RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		slog.Info("http_request", "request_id", requestID, "method", c.Request.Method,
			"route", route, "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}

func RequestID(c *gin.Context) string {
	id, _ := c.Get(RequestIDKey)
	value, _ := id.(string)
	return value
}

func LogHTTPError(c *gin.Context, code string, status int, err error) {
	if status < http.StatusInternalServerError {
		return
	}
	slog.Error("http_error", "request_id", RequestID(c), "code", code,
		"status", status, "error_type", fmt.Sprintf("%T", err))
}
