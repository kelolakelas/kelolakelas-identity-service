package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Tenant context failures are separated so handlers can answer with a stable
// status code instead of leaking a parse error.
var (
	errTenantContextMissing = errors.New("tenant context is missing")
	errTenantContextInvalid = errors.New("tenant context is invalid")
)

// tenantIDFromContext resolves the active tenant exclusively from the verified
// JWT claim set by AuthMiddleware.
//
// Client-supplied headers are never trusted. The api-gateway forwards inbound
// headers unchanged and only injects X-Tenant-ID when the token carries a
// tenant, so a parent token (which legitimately has no tenant claim) could
// otherwise name any tenant and read the member directory, mutate roles, or
// reach tenant settings.
func tenantIDFromContext(c *gin.Context) (uuid.UUID, error) {
	value, exists := c.Get("tenant_id")
	if !exists {
		return uuid.Nil, errTenantContextMissing
	}

	switch tenant := value.(type) {
	case uuid.UUID:
		if tenant == uuid.Nil {
			return uuid.Nil, errTenantContextMissing
		}
		return tenant, nil
	case string:
		if tenant == "" {
			return uuid.Nil, errTenantContextMissing
		}
		parsed, err := uuid.Parse(tenant)
		if err != nil {
			return uuid.Nil, errTenantContextInvalid
		}
		// An all-zero UUID means "no tenant" whichever representation produced
		// it, so it is reported the same way as a nil uuid.UUID claim.
		if parsed == uuid.Nil {
			return uuid.Nil, errTenantContextMissing
		}
		return parsed, nil
	default:
		return uuid.Nil, errTenantContextInvalid
	}
}

// writeTenantError maps tenant context failures onto HTTP responses.
//
// A caller without a tenant claim (for example a parent) is rejected with 403
// because the route requires a tenant scope the token does not grant; that is
// an authorization decision, not a malformed request. It is deliberately not a
// 400 (the input may be perfectly well formed) and not a 401 (AuthMiddleware
// already rejected unusable tokens), so callers can tell the three apart.
func writeTenantError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errTenantContextInvalid):
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  "error",
			"message": "Invalid tenant context",
			"data":    nil,
		})
	default:
		c.JSON(http.StatusForbidden, gin.H{
			"status":  "error",
			"message": "Tenant context is required for this operation",
			"data":    nil,
		})
	}
}
