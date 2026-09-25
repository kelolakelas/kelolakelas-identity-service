package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type InvitationHandler struct {
	invitationUsecase usecase.InvitationUsecase
	authUsecase       domain.AuthUsecase
}

func NewInvitationHandler(invitationUsecase usecase.InvitationUsecase, authUsecase domain.AuthUsecase) *InvitationHandler {
	return &InvitationHandler{
		invitationUsecase: invitationUsecase,
		authUsecase:       authUsecase,
	}
}

type CreateInvitationPayload struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
	Email  string    `json:"email" binding:"required,email"`
}

// InvitationResponse intentionally never serializes the bearer token.
type InvitationResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	RoleID    uuid.UUID `json:"role_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Status    string    `json:"status"`
	EmailSent bool      `json:"email_sent"`
}

func invitationResponse(invitation domain.TenantInvitation) InvitationResponse {
	status := "active"
	if !time.Now().Before(invitation.ExpiresAt) {
		status = "expired"
	}
	return InvitationResponse{
		ID: invitation.ID, Email: invitation.Email, RoleID: invitation.RoleID,
		ExpiresAt: invitation.ExpiresAt, Status: status, EmailSent: invitation.EmailSent,
	}
}

type RegisterInvitedUserPayload struct {
	Token     string `json:"token" binding:"required"`
	FirstName string `json:"first_name" binding:"required,max=255"`
	LastName  string `json:"last_name" binding:"required,max=255"`
	Password  string `json:"password" binding:"required,min=6"`
}

// createInvitationMessage renders the 201 response message. A stored
// invitation is a success either way; the message states whether the email
// actually went out so the tenant knows when to resend it manually.
func createInvitationMessage(emailSent bool) string {
	if emailSent {
		return "Invitation created and email sent successfully"
	}
	return "Invitation created but the email could not be sent. Please ask the member to contact support or resend the invitation later."
}

// CreateInvitation godoc
// @Summary Create tenant invitation
// @Description Invite a new member to join the tenant carried by the caller's access token
// @Description The tenant is resolved from the verified JWT claim only; any X-Tenant-ID header is ignored
// @Tags Invitations
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateInvitationPayload true "Create invitation payload"
// @Success 201 {object} domain.HTTPResponse{data=InvitationResponse}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/invitations [post]
func (h *InvitationHandler) CreateInvitation(c *gin.Context) {
	tenantIDVal, exists := c.Get("tenant_id")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Tenant context required",
			"data":    nil,
		})
		return
	}

	tenantID, ok := tenantIDVal.(uuid.UUID)
	if !ok || tenantID == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid tenant ID in token",
			"data":    nil,
		})
		return
	}

	var payload CreateInvitationPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	invitation, err := h.invitationUsecase.CreateInvitation(c.Request.Context(), tenantID, extractCallerRoleID(c), payload.RoleID, payload.Email)
	if err != nil {
		if errors.Is(err, domain.ErrCreatorGrantForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Creator role requires platform approval", "data": nil})
			return
		}
		if errors.Is(err, domain.ErrPermissionDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Permission denied",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrAlreadyTenantMember) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": "User is already a member of this tenant",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "User with this email already exists", "data": nil})
			return
		}
		if errors.Is(err, domain.ErrInvitationRoleInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Role does not belong to this tenant or is not a system role",
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to create invitation: " + err.Error(),
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": createInvitationMessage(invitation.EmailSent),
		"data":    invitationResponse(*invitation),
	})
}

func invitationTenantID(c *gin.Context) (uuid.UUID, bool) {
	value, exists := c.Get("tenant_id")
	id, ok := value.(uuid.UUID)
	return id, exists && ok && id != uuid.Nil
}

// ListInvitations godoc
// @Summary List unredeemed tenant invitations
// @Tags Invitations
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=[]InvitationResponse}
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/invitations [get]
func (h *InvitationHandler) ListInvitations(c *gin.Context) {
	tenantID, ok := invitationTenantID(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Tenant context required", "data": nil})
		return
	}
	invitations, err := h.invitationUsecase.ListInvitations(c.Request.Context(), tenantID, extractCallerRoleID(c))
	if err != nil {
		if errors.Is(err, domain.ErrPermissionDenied) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Permission denied", "data": nil})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to list invitations", "data": nil})
		}
		return
	}
	data := make([]InvitationResponse, 0, len(invitations))
	for _, invitation := range invitations {
		data = append(data, invitationResponse(invitation))
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": data})
}

// RevokeInvitation godoc
// @Summary Revoke an unredeemed tenant invitation
// @Tags Invitations
// @Produce json
// @Security BearerAuth
// @Param id path string true "Invitation ID"
// @Success 204
// @Failure 400 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/invitations/{id} [delete]
func (h *InvitationHandler) RevokeInvitation(c *gin.Context) {
	tenantID, ok := invitationTenantID(c)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Tenant context required", "data": nil})
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid invitation ID", "data": nil})
		return
	}
	if err := h.invitationUsecase.RevokeInvitation(c.Request.Context(), tenantID, extractCallerRoleID(c), id); err != nil {
		switch {
		case errors.Is(err, domain.ErrPermissionDenied):
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Permission denied", "data": nil})
		case errors.Is(err, domain.ErrInvitationNotFound):
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Invitation not found", "data": nil})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to revoke invitation", "data": nil})
		}
		return
	}
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

// VerifyInvitation godoc
// @Summary Verify invitation token
// @Description Verify validity of an invitation token
// @Tags Invitations
// @Accept json
// @Produce json
// @Param token query string true "Invitation token"
// @Success 200 {object} domain.HTTPResponse{data=domain.TenantInvitation}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/invitations/verify [get]
func (h *InvitationHandler) VerifyInvitation(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Token parameter is required",
			"data":    nil,
		})
		return
	}

	invitation, err := h.invitationUsecase.VerifyInvitation(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, domain.ErrInvitationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "Invitation not found",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrInvitationExpired) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Invitation token has expired",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrInvitationUsed) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Invitation token has already been used",
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to verify invitation",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Invitation is valid",
		"data":    invitation,
	})
}

// RegisterInvitedUser godoc
// @Summary Register user via invitation
// @Description Complete user registration using a valid invitation token
// @Tags Invitations
// @Accept json
// @Produce json
// @Param request body RegisterInvitedUserPayload true "Register invited user payload"
// @Success 201 {object} domain.HTTPResponse{data=domain.User}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/invitations/register [post]
func (h *InvitationHandler) RegisterInvitedUser(c *gin.Context) {
	var payload RegisterInvitedUserPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	if len(payload.Password) > 72 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Password exceeds 72 bytes", "data": nil})
		return
	}

	user, err := h.authUsecase.RegisterInvitedUser(c.Request.Context(), payload.Token, payload.FirstName, payload.LastName, payload.Password)
	if err != nil {
		if errors.Is(err, domain.ErrCreatorGrantForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Creator role requires platform approval", "data": nil})
			return
		}
		if errors.Is(err, domain.ErrInvitationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "Invitation not found",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrInvitationExpired) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Invitation token has expired",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrInvitationUsed) {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Invitation token has already been used",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": "User with this email already exists",
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to register invited user: " + err.Error(),
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "Invited user registered successfully",
		"data":    user,
	})
}
