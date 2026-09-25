package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"net/http"
)

type CreatorDecisionHandler struct {
	usecase *usecase.CreatorDecisionUsecase
}

func NewCreatorDecisionHandler(u *usecase.CreatorDecisionUsecase) *CreatorDecisionHandler {
	return &CreatorDecisionHandler{usecase: u}
}

type rejectCreatorPayload struct {
	Reason string `json:"reason" binding:"required"`
}

// Approve godoc
// @Summary Approve a pending Creator request as active platform admin
// @Tags Creator Requests
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Success 200 {object} domain.HTTPResponse{data=domain.CreatorRequest}
// @Failure 400,401,403,404,409,500 {object} domain.ErrorResponse
// @Router /api/v1/platform/creator-requests/{id}/approve [post]
func (h *CreatorDecisionHandler) Approve(c *gin.Context) { h.decide(c, true) }

// Reject godoc
// @Summary Reject a pending Creator request as active platform admin
// @Tags Creator Requests
// @Security BearerAuth
// @Param id path string true "Request ID"
// @Param request body rejectCreatorPayload true "Rejection reason"
// @Success 200 {object} domain.HTTPResponse{data=domain.CreatorRequest}
// @Failure 400,401,403,404,409,500 {object} domain.ErrorResponse
// @Router /api/v1/platform/creator-requests/{id}/reject [post]
func (h *CreatorDecisionHandler) Reject(c *gin.Context) { h.decide(c, false) }

func (h *CreatorDecisionHandler) decide(c *gin.Context, approve bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request ID"})
		return
	}
	actor, ok := c.Get("user_id")
	actorID, valid := actor.(uuid.UUID)
	if !ok || !valid || actorID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required"})
		return
	}
	var reason string
	if !approve {
		var payload rejectCreatorPayload
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Rejection reason required"})
			return
		}
		reason = payload.Reason
	}
	result, err := h.usecase.Decide(c.Request.Context(), id, actorID, approve, reason)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, domain.ErrCreatorRequestForbidden):
			status = http.StatusForbidden
		case errors.Is(err, domain.ErrCreatorRequestInvalid):
			status = http.StatusBadRequest
		case errors.Is(err, domain.ErrCreatorRequestNotFound):
			status = http.StatusNotFound
		case errors.Is(err, domain.ErrCreatorRequestDecided), errors.Is(err, domain.ErrCreatorRequestStale), errors.Is(err, domain.ErrCreatorTargetAlreadyCreator):
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"status": "error", "message": http.StatusText(status), "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Creator request decided", "data": result})
}
