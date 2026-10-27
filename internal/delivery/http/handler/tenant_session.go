package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type TenantSessionHandler struct{ sessions *usecase.TenantSessions }

func NewTenantSessionHandler(sessions *usecase.TenantSessions) *TenantSessionHandler {
	return &TenantSessionHandler{sessions: sessions}
}

func tenantSessionError(c *gin.Context, err error) {
	status, message := http.StatusServiceUnavailable, "Tenant session unavailable"
	if errors.Is(err, domain.ErrTenantSessionInvalid) {
		status, message = http.StatusUnauthorized, "Invalid or expired session"
	}
	if errors.Is(err, domain.ErrTenantSessionForbidden) {
		status, message = http.StatusForbidden, "Active own tenant membership required"
	}
	if status == http.StatusServiceUnavailable {
		slog.ErrorContext(c.Request.Context(), "tenant session failed", "error", err)
	}
	c.JSON(status, gin.H{"status": "error", "message": message})
}

func (h *TenantSessionHandler) List(c *gin.Context) {
	choices, err := h.sessions.List(c.Request.Context(), bearer(c))
	if err != nil {
		tenantSessionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": choices})
}

func (h *TenantSessionHandler) Switch(c *gin.Context) {
	var payload struct {
		TenantID uuid.UUID `json:"tenant_id" binding:"required"`
	}
	if c.ShouldBindJSON(&payload) != nil || payload.TenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Valid tenant_id required"})
		return
	}
	token, choice, err := h.sessions.Switch(c.Request.Context(), bearer(c), payload.TenantID)
	if err != nil {
		tenantSessionError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"token": token, "membership": choice}})
}
