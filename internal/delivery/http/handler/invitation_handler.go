package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/tutorin-id/tutorin-identity-service/internal/domain"
	"github.com/tutorin-id/tutorin-identity-service/internal/usecase"
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

type RegisterInvitedUserPayload struct {
	Token     string `json:"token" binding:"required"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
	Password  string `json:"password" binding:"required,min=6"`
}

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

	invitation, err := h.invitationUsecase.CreateInvitation(c.Request.Context(), tenantID, payload.RoleID, payload.Email)
	if err != nil {
		if errors.Is(err, domain.ErrAlreadyTenantMember) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": "User is already a member of this tenant",
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
		"message": "Invitation created and email sent successfully",
		"data":    invitation,
	})
}

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

	user, err := h.authUsecase.RegisterInvitedUser(c.Request.Context(), payload.Token, payload.FirstName, payload.LastName, payload.Password)
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
