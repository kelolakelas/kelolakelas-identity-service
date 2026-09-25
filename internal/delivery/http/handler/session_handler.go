package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

// SessionCheck is intentionally not proxied as a public gateway route. The
// gateway submits the caller's signed JWT, so neither a user ID nor an iat
// supplied outside the signature can select a different user's boundary.
func SessionCheck(tokens *jwt.JWTService, store domain.PasswordResetRepository, platform ...*usecase.PlatformAuth) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Status(http.StatusUnauthorized)
			return
		}
		claims, err := tokens.ValidateToken(parts[1])
		if err != nil || claims.UserID == uuid.Nil || claims.IssuedAt == nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		boundary, err := store.SessionValidAfter(c.Request.Context(), claims.UserID)
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		if boundary != nil && claims.IssuedAt.Time.Before(*boundary) {
			c.Status(http.StatusUnauthorized)
			return
		}
		if claims.IsPlatformAdmin || claims.PlatformPending {
			if claims.PlatformPending || claims.PlatformFactorVersion <= 0 || len(platform) == 0 || platform[0] == nil {
				c.Status(http.StatusUnauthorized)
				return
			}
			if err := platform[0].CheckVersion(c.Request.Context(), claims.UserID, claims.PlatformFactorVersion); err != nil {
				if errors.Is(err, usecase.ErrPlatformForbidden) {
					c.Status(http.StatusUnauthorized)
				} else {
					c.Status(http.StatusServiceUnavailable)
				}
				return
			}
		}
		c.Status(http.StatusNoContent)
	}
}
