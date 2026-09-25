package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type activePlatformUsers struct{ id uuid.UUID }

func (u activePlatformUsers) Create(context.Context, *domain.User) error { panic("unused") }
func (u activePlatformUsers) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if id == u.id {
		return &domain.User{ID: id}, nil
	}
	return nil, domain.ErrUserNotFound
}
func (activePlatformUsers) GetByEmail(context.Context, string) (*domain.User, error) { panic("unused") }
func (activePlatformUsers) Update(context.Context, *domain.User) error               { panic("unused") }
func (activePlatformUsers) Delete(context.Context, uuid.UUID) error                  { panic("unused") }
func (activePlatformUsers) RegisterTenantTx(context.Context, *domain.User, *domain.Tenant) (*domain.TenantMember, error) {
	panic("unused")
}
func (activePlatformUsers) RegisterInvitedUserTx(context.Context, string, string, string, string) (*domain.User, error) {
	panic("unused")
}
func (activePlatformUsers) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	panic("unused")
}
func (activePlatformUsers) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	panic("unused")
}

type activePlatformAssignments struct {
	active bool
	err    error
	checks int
}

func (a *activePlatformAssignments) IsActive(context.Context, uuid.UUID) (bool, error) {
	a.checks++
	return a.active, a.err
}

type middlewareFactorStore struct{}

func (middlewareFactorStore) Version(context.Context, uuid.UUID) (int64, error) { return 1, nil }
func (middlewareFactorStore) AssignmentVersion(context.Context, uuid.UUID) (int64, error) {
	return 1, nil
}
func (middlewareFactorStore) Begin(context.Context, uuid.UUID, string, []byte, []byte, time.Time) (int64, error) {
	panic("unused")
}
func (middlewareFactorStore) Consume(context.Context, uuid.UUID, string, []byte, time.Time, func([]byte) (bool, error)) (bool, int64, error) {
	panic("unused")
}

func TestRequireActivePlatformEnforcesLiveAssignmentAndPrincipalBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := uuid.New()
	tokens := jwt.NewJWTService("platform-test-secret", time.Hour)
	platformToken, err := tokens.GenerateVerifiedPlatformToken(id, "admin@example.test", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	tenantToken, err := tokens.GenerateToken(id, "tenant@example.test", uuid.New(), uuid.New(), uuid.New(), false)
	if err != nil {
		t.Fatal(err)
	}
	parentToken, err := tokens.GenerateToken(id, "parent@example.test", uuid.Nil, uuid.Nil, uuid.Nil, true)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name      string
		token     string
		active    bool
		err       error
		want      int
		wantCheck int
	}{
		{name: "active platform assignment", token: platformToken, active: true, want: http.StatusNoContent, wantCheck: 1},
		{name: "revoked platform assignment", token: platformToken, want: http.StatusForbidden, wantCheck: 1},
		{name: "tenant token", token: tenantToken, active: true, want: http.StatusForbidden},
		{name: "parent token", token: parentToken, active: true, want: http.StatusForbidden},
		{name: "assignment store outage fails closed", token: platformToken, err: errors.New("database unavailable"), want: http.StatusServiceUnavailable, wantCheck: 1},
		{name: "invalid token", token: "not-a-token", active: true, want: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			assignments := &activePlatformAssignments{active: test.active, err: test.err}
			auth := usecase.NewPlatformAuth(activePlatformUsers{id: id}, assignments, tokens).WithFactor(middlewareFactorStore{})
			handlerCalled := false
			router := gin.New()
			router.GET("/api/v1/platform/configurations", AuthMiddleware(tokens), RequireActivePlatform(auth), func(c *gin.Context) {
				handlerCalled = true
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/api/v1/platform/configurations", nil)
			request.Header.Set("Authorization", "Bearer "+test.token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
			if handlerCalled != (test.want == http.StatusNoContent) {
				t.Fatalf("handler called=%v for status %d", handlerCalled, response.Code)
			}
			if assignments.checks != test.wantCheck {
				t.Fatalf("assignment checks=%d want=%d", assignments.checks, test.wantCheck)
			}
		})
	}
}
