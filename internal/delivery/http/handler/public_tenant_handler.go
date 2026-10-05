package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

type publicTenantReader interface {
	GetTenantByID(context.Context, uuid.UUID) (*domain.Tenant, error)
}
type publicCatalogEvaluator interface {
	Evaluate(context.Context) (domain.PublicCatalogPolicyEvaluated, error)
}

// PublicTenantProfile deliberately excludes internal settings and unstructured about data.
type PublicTenantProfile struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Address   *string   `json:"address,omitempty"`
	Latitude  *float64  `json:"latitude,omitempty"`
	Longitude *float64  `json:"longitude,omitempty"`
}

func PublicTenantHandler(reader publicTenantReader, policy publicCatalogEvaluator) gin.HandlerFunc {
	return func(c *gin.Context) {
		missing := func() {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Tenant not found", "data": nil})
		}
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			missing()
			return
		}
		state, err := policy.Evaluate(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Public catalog unavailable", "data": nil})
			return
		}
		if !state.Open {
			missing()
			return
		}
		tenant, err := reader.GetTenantByID(c.Request.Context(), id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			missing()
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch tenant profile", "data": nil})
			return
		}
		if tenant == nil || tenant.Status != "active" || tenant.DeletedAt.Valid {
			missing()
			return
		}
		address := tenant.AddressFormatted
		if address == nil || *address == "" {
			address = tenant.Address
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "data": PublicTenantProfile{ID: tenant.ID, Name: tenant.Name, Address: address, Latitude: tenant.Latitude, Longitude: tenant.Longitude}})
	}
}
