package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

// Keep the response shape referenced in Swagger and the implementation contract.
var _ domain.PlatformCreatorRequest

type PlatformCreatorRequestHandler struct {
	repo repository.PlatformCreatorRequestRepository
}

func NewPlatformCreatorRequestHandler(repo repository.PlatformCreatorRequestRepository) *PlatformCreatorRequestHandler {
	return &PlatformCreatorRequestHandler{repo: repo}
}

// List godoc
// @Summary List pending Creator requests across tenants for active platform admins
// @Tags Creator Requests
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=[]domain.PlatformCreatorRequest}
// @Failure 401,403,503,500 {object} domain.ErrorResponse
// @Router /api/v1/platform/creator-requests [get]
func (h *PlatformCreatorRequestHandler) List(c *gin.Context) {
	requests, err := h.repo.ListPlatform(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch Creator requests", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Creator requests fetched", "data": requests})
}
