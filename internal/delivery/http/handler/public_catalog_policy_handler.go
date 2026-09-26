package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// PublicCatalogPolicyHandler exposes the KEL-98 public catalog visibility
// policy to platform admins. History, applied reports, and rollback stay on the
// generic configuration endpoints (application "academic", key
// "PUBLIC_CATALOG_OPEN", environment "platform"); these routes are shortcuts
// that read the effective state and append desired versions.
type PublicCatalogPolicyHandler struct {
	policy domain.PublicCatalogPolicy
}

func NewPublicCatalogPolicyHandler(policy domain.PublicCatalogPolicy) *PublicCatalogPolicyHandler {
	return &PublicCatalogPolicyHandler{policy: policy}
}

// PublicCatalogPolicy godoc
// @Summary Read the public catalog visibility policy
// @Description Returns the effective open/closed state that academic enforces for the public catalog list and detail, with the applied and desired configuration versions that decided it.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.PublicCatalogPolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/catalog-policy [get]
func (h *PublicCatalogPolicyHandler) Get(c *gin.Context) {
	evaluated, err := h.policy.Evaluate(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Public catalog policy unavailable", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": evaluated})
}

// ClosePublicCatalog godoc
// @Summary Close the public catalog
// @Description Appends a desired configuration version that hides every class from the public catalog list and detail. Classes and tenants are not changed. The catalog closes once the operator acknowledges the version as applied.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.PublicCatalogPolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/catalog-policy/close [post]
func (h *PublicCatalogPolicyHandler) Close(c *gin.Context) {
	h.mutate(c, false)
}

// OpenPublicCatalog godoc
// @Summary Reopen the public catalog
// @Description Appends a desired configuration version that shows the public catalog again under the existing published/active rules. The catalog reopens once the operator acknowledges the version as applied.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.PublicCatalogPolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/catalog-policy/open [post]
func (h *PublicCatalogPolicyHandler) Open(c *gin.Context) {
	h.mutate(c, true)
}

func (h *PublicCatalogPolicyHandler) mutate(c *gin.Context, open bool) {
	operator, ok := c.Get("user_id")
	operatorID, valid := operator.(uuid.UUID)
	if !ok || !valid || operatorID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
		return
	}
	var evaluated domain.PublicCatalogPolicyEvaluated
	var err error
	if open {
		evaluated, err = h.policy.Open(c.Request.Context(), operatorID)
	} else {
		evaluated, err = h.policy.Close(c.Request.Context(), operatorID)
	}
	if errors.Is(err, domain.ErrConfigurationVersionConflict) {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "Public catalog policy changed concurrently; read it again and retry", "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Public catalog policy was not updated", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": evaluated})
}
