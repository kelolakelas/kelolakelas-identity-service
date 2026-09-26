package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// RequestID returns the validated correlation ID, or empty for non-HTTP work.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "req-" + strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// RequestLog correlates access without logging headers, query strings, payloads or path parameters.
func RequestLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if !validRequestID(id) {
			id = newRequestID()
		}
		c.Request.Header.Set(requestIDHeader, id)
		c.Header(requestIDHeader, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, id))
		start := time.Now()
		c.Next()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		slog.InfoContext(c.Request.Context(), "http request", "request_id", id, "method", c.Request.Method, "route", path, "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}
