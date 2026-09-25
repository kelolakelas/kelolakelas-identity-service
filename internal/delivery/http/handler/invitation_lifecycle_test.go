package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type lifecycleHandlerStub struct {
	deliveryStatusInvitationUsecase
	list               []domain.TenantInvitation
	listErr, revokeErr error
	tenant, role       uuid.UUID
}

func (s *lifecycleHandlerStub) ListInvitations(_ context.Context, tenant, role uuid.UUID) ([]domain.TenantInvitation, error) {
	s.tenant, s.role = tenant, role
	return s.list, s.listErr
}
func (s *lifecycleHandlerStub) RevokeInvitation(_ context.Context, tenant, role, id uuid.UUID) error {
	s.tenant, s.role = tenant, role
	return s.revokeErr
}

func TestInvitationHandlersDoNotExposeTokensAndHandleErrors(t *testing.T) {
	tenant, role := uuid.New(), uuid.New()
	invitation := domain.TenantInvitation{ID: uuid.New(), Email: "member@example.com", RoleID: role, Token: "secret-invitation-token", ExpiresAt: time.Now().Add(time.Hour), EmailSent: true}
	create := performCreateInvitation(&deliveryStatusInvitationUsecase{invitation: &invitation})
	if create.Code != 201 || strings.Contains(create.Body.String(), invitation.Token) || strings.Contains(create.Body.String(), `"token"`) {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	stub := &lifecycleHandlerStub{list: []domain.TenantInvitation{invitation}}
	handler := NewInvitationHandler(stub, nil)
	call := func(method, path string, invoke func(*gin.Context)) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(method, path, nil)
		ctx.Set("tenant_id", tenant)
		ctx.Set("role_id", role)
		ctx.Params = gin.Params{{Key: "id", Value: invitation.ID.String()}}
		invoke(ctx)
		return recorder
	}
	result := call(http.MethodGet, "/api/v1/invitations", handler.ListInvitations)
	if result.Code != 200 || strings.Contains(result.Body.String(), invitation.Token) || strings.Contains(result.Body.String(), `"token"`) || stub.tenant != tenant || stub.role != role {
		t.Fatalf("list: %d %s tenant=%s role=%s", result.Code, result.Body.String(), stub.tenant, stub.role)
	}
	var body struct {
		Data []InvitationResponse `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &body); err != nil || len(body.Data) != 1 || body.Data[0].Status != "active" {
		t.Fatalf("list: %+v %v", body, err)
	}
	stub.listErr = domain.ErrPermissionDenied
	if got := call(http.MethodGet, "/api/v1/invitations", handler.ListInvitations); got.Code != 403 {
		t.Fatalf("list denied: %d", got.Code)
	}
	stub.revokeErr = domain.ErrPermissionDenied
	if got := call(http.MethodDelete, "/api/v1/invitations/id", handler.RevokeInvitation); got.Code != 403 {
		t.Fatalf("revoke denied: %d", got.Code)
	}
	stub.revokeErr = domain.ErrInvitationNotFound
	if got := call(http.MethodDelete, "/api/v1/invitations/id", handler.RevokeInvitation); got.Code != 404 {
		t.Fatalf("revoke foreign: %d", got.Code)
	}
	stub.revokeErr = nil
	if got := call(http.MethodDelete, "/api/v1/invitations/id", handler.RevokeInvitation); got.Code != 204 {
		t.Fatalf("revoke: %d", got.Code)
	}
	if got := call(http.MethodDelete, "/api/v1/invitations/id", func(c *gin.Context) { c.Params[0].Value = "invalid"; handler.RevokeInvitation(c) }); got.Code != 400 {
		t.Fatalf("invalid ID: %d", got.Code)
	}
}

func TestCreateInvitationExistingUserReturnsConflict(t *testing.T) {
	result := performCreateInvitation(&deliveryStatusInvitationUsecase{err: domain.ErrUserAlreadyExists})
	if result.Code != http.StatusConflict || !strings.Contains(result.Body.String(), "already exists") {
		t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
	}
}
