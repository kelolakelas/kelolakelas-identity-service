package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

func AuthMiddleware(jwtService *jwt.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Authorization header required",
				"data":    nil,
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Invalid Authorization header format",
				"data":    nil,
			})
			c.Abort()
			return
		}

		claims, err := jwtService.ValidateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Invalid or expired token",
				"data":    nil,
			})
			c.Abort()
			return
		}

		// Pending and legacy platform JWTs are not ordinary principals either.
		// Reject them before any protected identity handler sees their claims.
		if claims.PlatformPending || (claims.IsPlatformAdmin && claims.PlatformFactorVersion <= 0) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid or expired token", "data": nil})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("tenant_id", claims.TenantID)
		c.Set("role_id", claims.RoleID)
		c.Set("member_id", claims.MemberID)
		c.Set("is_platform_admin", claims.IsPlatformAdmin && claims.PlatformFactorVersion > 0 && !claims.PlatformPending)
		c.Set("platform_factor_version", claims.PlatformFactorVersion)
		// Permission checks require the token's user (and membership, when the token carries
		// the member_id claim) to still hold an active membership with the token's role
		// (KEL-76), so the verified caller travels with the request context to the usecases.
		c.Request = c.Request.WithContext(domain.WithCaller(c.Request.Context(), domain.Caller{UserID: claims.UserID, MemberID: claims.MemberID}))

		c.Next()
	}
}
