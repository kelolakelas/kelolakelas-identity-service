package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type creatorHandlerStore struct {
	calls  int
	err    error
	tenant uuid.UUID
}

func (s *creatorHandlerStore) Create(_ context.Context, tenant, _, _ uuid.UUID, email string, _ *uuid.UUID, _ string) (*domain.CreatorRequest, error) {
	s.calls++
	s.tenant = tenant
	if s.err != nil {
		return nil, s.err
	}
	return &domain.CreatorRequest{TenantID: tenant, TargetEmail: email, Status: "pending"}, nil
}
func (s *creatorHandlerStore) List(_ context.Context, tenant, _, _ uuid.UUID) ([]domain.CreatorRequest, error) {
	s.calls++
	s.tenant = tenant
	if s.err != nil {
		return nil, s.err
	}
	return []domain.CreatorRequest{}, nil
}
func TestCreatorRequestHandlerContextAndErrors(t *testing.T) {
	tenant := uuid.New()
	for _, tc := range []struct {
		name, method, body        string
		contextTenant, user, role uuid.UUID
		repoErr                   error
		want                      int
		calls                     int
	}{
		{name: "create", method: http.MethodPost, body: `{"target_email":"new@example.com","reason":"why","tenant_id":"` + uuid.New().String() + `"}`, contextTenant: tenant, user: uuid.New(), role: uuid.New(), want: 201, calls: 1},
		{name: "list", method: http.MethodGet, contextTenant: tenant, user: uuid.New(), role: uuid.New(), want: 200, calls: 1},
		{name: "parent", method: http.MethodGet, want: 403},
		{name: "missing role", method: http.MethodGet, contextTenant: tenant, user: uuid.New(), want: 403},
		{name: "not Creator", method: http.MethodGet, contextTenant: tenant, user: uuid.New(), role: uuid.New(), repoErr: domain.ErrCreatorRequestForbidden, want: 403, calls: 1},
		{name: "duplicate", method: http.MethodPost, body: `{"target_email":"new@example.com","reason":"why"}`, contextTenant: tenant, user: uuid.New(), role: uuid.New(), repoErr: domain.ErrCreatorRequestDuplicate, want: 409, calls: 1},
		{name: "already Creator", method: http.MethodPost, body: `{"target_email":"new@example.com","reason":"why"}`, contextTenant: tenant, user: uuid.New(), role: uuid.New(), repoErr: domain.ErrCreatorTargetAlreadyCreator, want: 409, calls: 1},
		{name: "bad payload", method: http.MethodPost, body: `{"reason":""}`, contextTenant: tenant, user: uuid.New(), role: uuid.New(), want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &creatorHandlerStore{err: tc.repoErr}
			h := NewCreatorRequestHandler(usecase.NewCreatorRequestUsecase(store))
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(tc.method, "/api/v1/creator-requests", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			if tc.contextTenant != uuid.Nil {
				c.Set("tenant_id", tc.contextTenant)
			}
			if tc.user != uuid.Nil {
				c.Set("user_id", tc.user)
			}
			if tc.role != uuid.Nil {
				c.Set("role_id", tc.role)
			}
			if tc.method == http.MethodPost {
				h.Create(c)
			} else {
				h.List(c)
			}
			if recorder.Code != tc.want || store.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, store.calls, recorder.Body.String())
			}
			if store.calls > 0 && store.tenant != tenant {
				t.Fatalf("tenant leaked from body: %v", store.tenant)
			}
		})
	}
}
