package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type PlatformHandler struct{ auth *usecase.PlatformAuth }

func NewPlatformHandler(auth *usecase.PlatformAuth) *PlatformHandler {
	return &PlatformHandler{auth: auth}
}

// Login is distinct from ordinary login: tenant membership cannot grant a
// platform token, and this route never creates an assignment.
func (h *PlatformHandler) Login(c *gin.Context) {
	var payload LoginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid login payload", "data": nil})
		return
	}
	token, err := h.auth.Login(c.Request.Context(), payload.Email, payload.Password)
	if errors.Is(err, domain.ErrInvalidCredentials) || errors.Is(err, usecase.ErrPlatformForbidden) {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid credentials", "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Authorization service unavailable", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"token": token}})
}

// Me re-reads the assignment on every request. The signed claim alone is
// insufficient: a revoked assignment must invalidate an otherwise live JWT.
func (h *PlatformHandler) Me(c *gin.Context) {
	admin, _ := c.Get("is_platform_admin")
	user, _ := c.Get("user_id")
	userID, ok := user.(uuid.UUID)
	if admin != true || !ok || userID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
		return
	}
	if err := h.auth.Check(c.Request.Context(), userID); err != nil {
		if errors.Is(err, usecase.ErrPlatformForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Authorization service unavailable", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"user_id": userID}})
}
