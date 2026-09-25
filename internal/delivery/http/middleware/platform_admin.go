package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

func RequireActivePlatform(auth *usecase.PlatformAuth) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, _ := c.Get("is_platform_admin")
		user, _ := c.Get("user_id")
		userID, ok := user.(uuid.UUID)
		if admin != true || !ok || userID == uuid.Nil {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
			c.Abort()
			return
		}
		if err := auth.Check(c.Request.Context(), userID); err != nil {
			if errors.Is(err, usecase.ErrPlatformForbidden) {
				c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
			} else {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Authorization service unavailable", "data": nil})
			}
			c.Abort()
			return
		}
		c.Next()
	}
}
