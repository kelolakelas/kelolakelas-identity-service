package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const probeTimeout = time.Second

// readinessHandler checks required storage and reports optional cache failures without exposing connection details.
func readinessHandler(db *sql.DB, cache redis.Cmdable) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), probeTimeout)
		defer cancel()
		components := gin.H{"database": "healthy"}
		status := "healthy"
		code := http.StatusOK
		if db == nil || db.PingContext(ctx) != nil {
			components["database"] = "unavailable"
			status, code = "unavailable", http.StatusServiceUnavailable
		}
		if cache != nil {
			if cache.Ping(ctx).Err() != nil {
				components["redis"] = "degraded"
				if code == http.StatusOK {
					status = "degraded"
				}
			} else {
				components["redis"] = "healthy"
			}
		} else {
			components["redis"] = "degraded"
			if code == http.StatusOK {
				status = "degraded"
			}
		}
		c.JSON(code, gin.H{"status": status, "service": "identity-service", "components": components})
	}
}
