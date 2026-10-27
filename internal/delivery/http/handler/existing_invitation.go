package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"net/http"
	"strings"
)

func AcceptExistingInvitation(service *usecase.ExistingInvitations) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var body struct {
			Token string `json:"token" binding:"required,max=255"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invitation token required"})
			return
		}
		authorization := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
		if len(authorization) != 2 || !strings.EqualFold(authorization[0], "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid session"})
			return
		}
		member, err := service.Accept(c.Request.Context(), body.Token, authorization[1])
		if err != nil {
			status, message := http.StatusInternalServerError, "Unable to accept invitation"
			switch {
			case errors.Is(err, domain.ErrTenantSessionInvalid):
				status, message = http.StatusUnauthorized, "Invalid session"
			case errors.Is(err, domain.ErrInvitationAccountMismatch):
				status, message = http.StatusForbidden, "Invitation belongs to another account. Sign in with the invited email."
			case errors.Is(err, domain.ErrTenantSessionForbidden), errors.Is(err, domain.ErrExistingTeacherRequired):
				status, message = http.StatusForbidden, "An active Teacher account and Teacher invitation are required"
			case errors.Is(err, domain.ErrInvitationMemberInactive):
				status, message = http.StatusConflict, "Existing membership is inactive or deleted. Contact the tenant administrator."
			case errors.Is(err, domain.ErrInvitationNotFound):
				status, message = http.StatusNotFound, "Invitation not found"
			case errors.Is(err, domain.ErrInvitationUsed):
				status, message = http.StatusConflict, "Invitation has already been used or revoked"
			case errors.Is(err, domain.ErrInvitationExpired):
				status, message = http.StatusBadRequest, "Invitation has expired"
			}
			c.JSON(status, gin.H{"status": "error", "message": message})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Invitation accepted. Select the tenant from your dashboard.", "data": gin.H{"tenant_id": member.TenantID, "member_id": member.ID, "role_id": member.RoleID}})
	}
}
