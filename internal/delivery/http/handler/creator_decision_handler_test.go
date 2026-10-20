package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type staleCreatorDecisionRepository struct{}

func (staleCreatorDecisionRepository) Decide(context.Context, uuid.UUID, uuid.UUID, bool, string) (*domain.CreatorRequest, *domain.TenantInvitation, string, error) {
	return nil, nil, "", domain.ErrCreatorRequestStale
}

func TestApproveStaleCreatorRequestReturnsConflict(t *testing.T) {
	router := gin.New()
	handler := NewCreatorDecisionHandler(usecase.NewCreatorDecisionUsecase(staleCreatorDecisionRepository{}, nil))
	router.POST("/creator-requests/:id/approve", func(c *gin.Context) {
		c.Set("user_id", uuid.New())
		handler.Approve(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/creator-requests/"+uuid.NewString()+"/approve", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
