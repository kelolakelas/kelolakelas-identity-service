package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type TenantHandler struct {
	usecase domain.TenantUsecase
}

func NewTenantHandler(usecase domain.TenantUsecase) *TenantHandler {
	return &TenantHandler{usecase: usecase}
}

func (h *TenantHandler) currentTenant(c *gin.Context) (uuid.UUID, bool) {
	id, err := extractTenantID(c)
	if err != nil || id == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Tenant ID is required", "data": nil})
		return uuid.Nil, false
	}
	return id, true
}

// GetSettings godoc
// @Summary Get tenant settings
// @Tags Tenant Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.Tenant}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/tenant/settings [get]
// @Router /api/v1/tenants/settings [get]
func (h *TenantHandler) GetSettings(c *gin.Context) {
	tenantID, ok := h.currentTenant(c)
	if !ok {
		return
	}
	tenant, err := h.usecase.GetTenantByID(c.Request.Context(), tenantID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"status": "error", "message": "Failed to fetch tenant settings", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Tenant settings fetched successfully", "data": tenant})
}

// UpdateSettings godoc
// @Summary Update tenant settings
// @Tags Tenant Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body domain.UpdateTenantSettingsRequest true "Tenant settings payload"
// @Success 200 {object} domain.HTTPResponse{data=domain.Tenant}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/tenant/settings [patch]
// @Router /api/v1/tenants/settings [patch]
func (h *TenantHandler) UpdateSettings(c *gin.Context) {
	tenantID, ok := h.currentTenant(c)
	if !ok {
		return
	}
	var req domain.UpdateTenantSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid tenant settings", "data": nil})
		return
	}
	tenant, err := h.usecase.UpdateTenantSettings(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to update tenant settings", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Tenant settings updated successfully", "data": tenant})
}
