package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type CreatorRequestHandler struct {
	usecase *usecase.CreatorRequestUsecase
}

func NewCreatorRequestHandler(uc *usecase.CreatorRequestUsecase) *CreatorRequestHandler {
	return &CreatorRequestHandler{usecase: uc}
}

type createCreatorRequestPayload struct {
	TargetEmail  string     `json:"target_email"`
	TargetUserID *uuid.UUID `json:"target_user_id"`
	Reason       string     `json:"reason" binding:"required"`
}

func creatorPrincipal(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	tenantID, err := tenantIDFromContext(c)
	if err != nil {
		writeTenantError(c, err)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	userID, ok := c.Get("user_id")
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Tenant member required", "data": nil})
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	uid, ok := userID.(uuid.UUID)
	roleID := extractCallerRoleID(c)
	if !ok || uid == uuid.Nil || roleID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Tenant member required", "data": nil})
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return tenantID, uid, roleID, true
}

func creatorRequestError(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, "Failed to process Creator request"
	switch {
	case errors.Is(err, domain.ErrCreatorRequestForbidden):
		status, message = http.StatusForbidden, "Active Creator membership required"
	case errors.Is(err, domain.ErrCreatorRequestInvalid):
		status, message = http.StatusBadRequest, "Invalid target or reason"
	case errors.Is(err, domain.ErrCreatorRequestDuplicate):
		status, message = http.StatusConflict, "Pending Creator request already exists"
	case errors.Is(err, domain.ErrCreatorTargetAlreadyCreator):
		status, message = http.StatusConflict, "Target is already a Creator"
	}
	c.JSON(status, gin.H{"status": "error", "message": message, "data": nil})
}

// Create godoc
// @Summary Request an additional tenant Creator
// @Tags Creator Requests
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body createCreatorRequestPayload true "Target and reason"
// @Success 201 {object} domain.HTTPResponse{data=domain.CreatorRequest}
// @Failure 400,401,403,409,500 {object} domain.ErrorResponse
// @Router /api/v1/creator-requests [post]
func (h *CreatorRequestHandler) Create(c *gin.Context) {
	tenantID, userID, roleID, ok := creatorPrincipal(c)
	if !ok {
		return
	}
	var payload createCreatorRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request payload", "data": nil})
		return
	}
	request, err := h.usecase.Create(c.Request.Context(), tenantID, userID, roleID, payload.TargetEmail, payload.TargetUserID, payload.Reason)
	if err != nil {
		creatorRequestError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "message": "Creator request created", "data": request})
}

// List godoc
// @Summary List Creator requests for the active tenant
// @Tags Creator Requests
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=[]domain.CreatorRequest}
// @Failure 401,403,500 {object} domain.ErrorResponse
// @Router /api/v1/creator-requests [get]
func (h *CreatorRequestHandler) List(c *gin.Context) {
	tenantID, userID, roleID, ok := creatorPrincipal(c)
	if !ok {
		return
	}
	requests, err := h.usecase.List(c.Request.Context(), tenantID, userID, roleID)
	if err != nil {
		creatorRequestError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Creator requests fetched", "data": requests})
}
