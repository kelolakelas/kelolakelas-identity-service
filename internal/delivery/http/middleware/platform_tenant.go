package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RejectTenantlessPlatform confines a platform-only principal to /platform.
// Existing parent and tenant handling remains with the individual handlers.
func RejectTenantlessPlatform() gin.HandlerFunc {
	return func(c *gin.Context) {
		admin, _ := c.Get("is_platform_admin")
		tenant, _ := c.Get("tenant_id")
		if admin == true && tenant == uuid.Nil {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Tenant access required", "data": nil})
			c.Abort()
		}
	}
}
