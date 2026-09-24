package handler

import (
	"bytes"
	"context"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/middleware"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type platformTestUsers struct{ user *domain.User }

func (u platformTestUsers) Create(context.Context, *domain.User) error { panic("unused") }
func (u platformTestUsers) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u.user.ID == id {
		return u.user, nil
	}
	return nil, domain.ErrUserNotFound
}
func (u platformTestUsers) GetByEmail(context.Context, string) (*domain.User, error) { panic("unused") }
func (u platformTestUsers) Update(context.Context, *domain.User) error               { panic("unused") }
func (u platformTestUsers) Delete(context.Context, uuid.UUID) error                  { panic("unused") }
func (u platformTestUsers) RegisterTenantTx(context.Context, *domain.User, *domain.Tenant) (*domain.TenantMember, error) {
	panic("unused")
}
func (u platformTestUsers) RegisterInvitedUserTx(context.Context, string, string, string, string) (*domain.User, error) {
	panic("unused")
}
func (u platformTestUsers) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	panic("unused")
}
func (u platformTestUsers) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	panic("unused")
}

type assignmentStub struct{ active bool }

func (a *assignmentStub) IsActive(_ context.Context, _ uuid.UUID) (bool, error) { return a.active, nil }

func TestPlatformMeRejectsStaleAndOrdinaryTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := jwt.NewJWTService("platform-test", time.Hour)
	id := uuid.New()
	users := platformTestUsers{user: &domain.User{ID: id, Email: "admin@example.test"}}
	assignment := &assignmentStub{active: true}
	h := NewPlatformHandler(usecase.NewPlatformAuth(users, assignment, tokens))
	router := gin.New()
	router.GET("/platform/me", middleware.AuthMiddleware(tokens), h.Me)
	admin, err := tokens.GeneratePlatformToken(id, "admin@example.test")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := tokens.GenerateToken(id, "admin@example.test", uuid.New(), uuid.New(), uuid.New(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		active      bool
		want        int
	}{
		{"admin", admin, true, 200}, {"revoked", admin, false, 403},
		{"ordinary tenant", ordinary, true, 403}, {"forged", "not-a-token", true, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assignment.active = tc.active
			req := httptest.NewRequest(http.MethodGet, "/platform/me", bytes.NewReader(nil))
			req.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
