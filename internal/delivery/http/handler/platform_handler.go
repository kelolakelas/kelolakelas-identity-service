package handler

import (
	"errors"
	"net/http"
	"strings"

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

func (h *PlatformHandler) StartFactor(c *gin.Context) {
	var payload struct {
		Purpose string `json:"purpose"`
	}
	if c.ShouldBindJSON(&payload) != nil || (payload.Purpose != "enroll" && payload.Purpose != "verify") {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request"})
		return
	}
	token, secret, err := h.auth.StartFactor(c.Request.Context(), bearer(c), payload.Purpose)
	if err != nil {
		platformFactorError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"challenge": token, "secret": secret}})
}
func (h *PlatformHandler) FinishFactor(c *gin.Context) {
	var payload struct {
		Purpose   string `json:"purpose"`
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if c.ShouldBindJSON(&payload) != nil || (payload.Purpose != "enroll" && payload.Purpose != "verify") {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request"})
		return
	}
	token, err := h.auth.FinishFactor(c.Request.Context(), bearer(c), payload.Challenge, payload.Code, payload.Purpose)
	if err != nil {
		platformFactorError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"token": token}})
}
func bearer(c *gin.Context) string {
	return strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
}
func platformFactorError(c *gin.Context, err error) {
	if errors.Is(err, usecase.ErrPlatformForbidden) {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid or expired challenge"})
		return
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Authorization service unavailable"})
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
	version, _ := c.Get("platform_factor_version")
	v, _ := version.(int64)
	if err := h.auth.CheckVersion(c.Request.Context(), userID, v); err != nil {
		if errors.Is(err, usecase.ErrPlatformForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "message": "Authorization service unavailable", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": gin.H{"user_id": userID}})
}
