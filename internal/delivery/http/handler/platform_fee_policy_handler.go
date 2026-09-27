package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// PlatformFeePolicyHandler exposes the KEL-99 platform fee policy to platform
// admins. History, applied reports, and rollback stay on the generic
// configuration endpoints (application "billing", key "PLATFORM_FEE_POLICY",
// environment "platform"); these routes read the effective rule and append a
// desired version with the owner-approved bounds enforced.
type PlatformFeePolicyHandler struct {
	policy domain.PlatformFeePolicy
}

func NewPlatformFeePolicyHandler(policy domain.PlatformFeePolicy) *PlatformFeePolicyHandler {
	return &PlatformFeePolicyHandler{policy: policy}
}

// PlatformFeePolicyRequest is the new fee rule. Both fields are required so a
// missing value is never read as 0.
type PlatformFeePolicyRequest struct {
	PercentBps *int64 `json:"percent_bps" binding:"required" example:"500"`
	FixedFee   *int64 `json:"fixed_fee" binding:"required" example:"1000"`
}

// PlatformFeePolicy godoc
// @Summary Read the platform fee policy
// @Description Returns the platform fee rule billing applies to new transactions (percent in basis points plus a fixed rupiah fee), whether an applied version exists, and the applied and desired configuration versions. Version 0 is the applied baseline of 0 bps + Rp0.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.PlatformFeePolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/fee-policy [get]
func (h *PlatformFeePolicyHandler) Get(c *gin.Context) {
	evaluated, err := h.policy.Evaluate(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Platform fee policy unavailable", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": evaluated})
}

// SetPlatformFeePolicy godoc
// @Summary Request a new platform fee policy
// @Description Appends a desired configuration version with percent_bps (0-2000) and fixed_fee (0-50000 rupiah). It applies only to transactions created after the operator acknowledges the version as applied; existing transactions keep their snapshot.
// @Tags Platform Configuration
// @Security BearerAuth
// @Accept json
// @Param request body PlatformFeePolicyRequest true "New platform fee rule"
// @Success 200 {object} domain.HTTPResponse{data=domain.PlatformFeePolicyEvaluated}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/fee-policy [post]
func (h *PlatformFeePolicyHandler) Set(c *gin.Context) {
	operator, ok := c.Get("user_id")
	operatorID, valid := operator.(uuid.UUID)
	if !ok || !valid || operatorID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
		return
	}
	var request PlatformFeePolicyRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.PercentBps == nil || request.FixedFee == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "percent_bps and fixed_fee are required integers", "data": nil})
		return
	}
	value := domain.PlatformFeePolicyValue{PercentBps: *request.PercentBps, FixedFee: *request.FixedFee}
	evaluated, err := h.policy.Set(c.Request.Context(), operatorID, value)
	if errors.Is(err, domain.ErrInvalidPlatformFeePolicy) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "percent_bps must be 0-2000 and fixed_fee must be 0-50000", "data": nil})
		return
	}
	if errors.Is(err, domain.ErrConfigurationVersionConflict) {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "Platform fee policy changed concurrently; read it again and retry", "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Platform fee policy was not updated", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": evaluated})
}
