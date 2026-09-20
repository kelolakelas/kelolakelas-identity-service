package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/middleware"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	svcjwt "github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

// KEL-16: identity tenant endpoints must resolve the tenant from the verified
// JWT claim only. These tests reproduce the reported attack: a parent token
// (which legitimately carries no tenant claim) is sent together with a foreign
// X-Tenant-ID header. The header must be ignored, the request must end in 403,
// and no use case may be reached, so no foreign tenant data is read or written.

const tenantContextTestSecret = "kel16-tenant-claim-only"

// foreignTenantHeader is deliberately a valid, different tenant UUID so that a
// handler still honouring the header would visibly succeed in the test.
var foreignTenantHeader = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// recordingMemberUsecase counts every call so the tests can prove the request
// was rejected before any repository work happened.
type recordingMemberUsecase struct {
	calls  int
	tenant uuid.UUID
}

func (m *recordingMemberUsecase) List(_ context.Context, tenantID uuid.UUID, _ domain.MemberQuery) (*domain.MemberListResponse, error) {
	m.calls++
	m.tenant = tenantID
	return &domain.MemberListResponse{Items: []domain.MemberResponse{}}, nil
}

func (m *recordingMemberUsecase) GetByID(_ context.Context, tenantID, _ uuid.UUID) (*domain.MemberResponse, error) {
	m.calls++
	m.tenant = tenantID
	return &domain.MemberResponse{}, nil
}

func (m *recordingMemberUsecase) UpdateRole(_ context.Context, tenantID, _, _, _ uuid.UUID) (*domain.MemberResponse, error) {
	m.calls++
	m.tenant = tenantID
	return &domain.MemberResponse{}, nil
}

func (m *recordingMemberUsecase) Delete(_ context.Context, tenantID, _, _ uuid.UUID) error {
	m.calls++
	m.tenant = tenantID
	return nil
}

func (m *recordingMemberUsecase) ListTutors(_ context.Context, tenantID uuid.UUID, _ domain.TutorQuery) (*domain.TutorListResponse, error) {
	m.calls++
	m.tenant = tenantID
	return &domain.TutorListResponse{Items: []domain.TutorResponse{}}, nil
}

// recordingRoleUsecase mirrors recordingMemberUsecase for the role endpoints.
type recordingRoleUsecase struct {
	calls  int
	tenant uuid.UUID
}

func (r *recordingRoleUsecase) FetchAllPermissions(context.Context) ([]domain.PermissionResponse, error) {
	return []domain.PermissionResponse{}, nil
}

func (r *recordingRoleUsecase) FetchTenantRoles(_ context.Context, tenantID uuid.UUID) ([]domain.RoleResponse, error) {
	r.calls++
	r.tenant = tenantID
	return []domain.RoleResponse{}, nil
}

func (r *recordingRoleUsecase) CreateCustomRole(_ context.Context, tenantID, _ uuid.UUID, _ *domain.CreateRoleRequest) (*domain.RoleResponse, error) {
	r.calls++
	r.tenant = tenantID
	return &domain.RoleResponse{}, nil
}

func (r *recordingRoleUsecase) UpdateCustomRole(_ context.Context, tenantID, _, _ uuid.UUID, _ *domain.UpdateRoleRequest) (*domain.RoleResponse, error) {
	r.calls++
	r.tenant = tenantID
	return &domain.RoleResponse{}, nil
}

func (r *recordingRoleUsecase) DeleteCustomRole(_ context.Context, tenantID, _, _ uuid.UUID) error {
	r.calls++
	r.tenant = tenantID
	return nil
}

// recordingTenantUsecase mirrors recordingMemberUsecase for the tenant
// settings and location endpoints.
type recordingTenantUsecase struct {
	calls  int
	tenant uuid.UUID
}

func (*recordingTenantUsecase) RegisterTenant(context.Context, *domain.RegisterTenantRequest) (*domain.RegisterTenantResponse, error) {
	return &domain.RegisterTenantResponse{}, nil
}

func (u *recordingTenantUsecase) GetTenantByID(_ context.Context, id uuid.UUID) (*domain.Tenant, error) {
	u.calls++
	u.tenant = id
	return &domain.Tenant{}, nil
}

func (u *recordingTenantUsecase) UpdateTenantSettings(_ context.Context, id, _ uuid.UUID, _ *domain.UpdateTenantSettingsRequest) (*domain.Tenant, error) {
	u.calls++
	u.tenant = id
	return &domain.Tenant{}, nil
}

func (u *recordingTenantUsecase) GetTenantLocation(_ context.Context, id uuid.UUID) (*domain.TenantLocation, error) {
	u.calls++
	u.tenant = id
	return &domain.TenantLocation{}, nil
}

func (u *recordingTenantUsecase) UpdateTenantLocation(_ context.Context, id, _ uuid.UUID, _ *domain.UpdateTenantLocationRequest) (*domain.TenantLocation, error) {
	u.calls++
	u.tenant = id
	return &domain.TenantLocation{}, nil
}

// tenantTestSecret is the JWT secret used by the router in these tests.
const tenantTestSecret = tenantContextTestSecret

// newTenantContextRouter builds the same protected route set that cmd/server
// registers, so the regression covers every tenant-scoped endpoint rather than
// a hand-picked subset.
func newTenantContextRouter(member domain.MemberUsecase, role usecase.RoleUsecase, tenant domain.TenantUsecase) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	jwtService := svcjwt.NewJWTService(tenantTestSecret, time.Hour)

	memberHandler := NewMemberHandler(member)
	roleHandler := NewRoleHandler(role)
	tenantHandler := NewTenantHandler(tenant)

	protected := router.Group("/api/v1")
	protected.Use(middleware.AuthMiddleware(jwtService))
	{
		protected.GET("/members", memberHandler.ListMembers)
		protected.GET("/tutors", memberHandler.ListTutors)
		protected.GET("/members/:id", memberHandler.GetMember)
		protected.PUT("/members/:id/role", memberHandler.UpdateMemberRole)
		protected.DELETE("/members/:id", memberHandler.DeleteMember)

		protected.GET("/tenant/settings", tenantHandler.GetSettings)
		protected.PATCH("/tenant/settings", tenantHandler.UpdateSettings)
		protected.GET("/tenants/settings", tenantHandler.GetSettings)
		protected.PATCH("/tenants/settings", tenantHandler.UpdateSettings)
		protected.GET("/tenant/settings/location", tenantHandler.GetLocation)
		protected.PUT("/tenant/settings/location", tenantHandler.UpdateLocation)

		protected.GET("/permissions", roleHandler.GetPermissions)
		protected.GET("/roles", roleHandler.GetRoles)
		protected.POST("/roles", roleHandler.CreateRole)
		protected.PUT("/roles/:id", roleHandler.UpdateRole)
		protected.DELETE("/roles/:id", roleHandler.DeleteRole)
	}
	return router
}

// signTenantToken mints a real HS256 token so AuthMiddleware accepts it and
// populates the context exactly like production does.
func signTenantToken(t *testing.T, tenantID uuid.UUID, isParent bool) string {
	t.Helper()
	claims := svcjwt.Claims{
		UserID:   uuid.New(),
		Email:    "caller@example.com",
		TenantID: tenantID,
		RoleID:   uuid.New(),
		IsParent: isParent,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(tenantTestSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func tenantRequest(router *gin.Engine, method, path, token string, body string, headerTenant *uuid.UUID) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	if headerTenant != nil {
		request.Header.Set("X-Tenant-ID", headerTenant.String())
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

type tenantEndpoint struct {
	name   string
	method string
	path   string
	body   string
}

// tenantEndpoints is the full tenant-scoped surface exposed by the identity
// service for member, role, and tenant administration.
func tenantEndpoints(roleID, memberID uuid.UUID) []tenantEndpoint {
	return []tenantEndpoint{
		{"list members", http.MethodGet, "/api/v1/members", ""},
		{"list tutors", http.MethodGet, "/api/v1/tutors", ""},
		{"get member", http.MethodGet, "/api/v1/members/" + memberID.String(), ""},
		{"update member role", http.MethodPut, "/api/v1/members/" + memberID.String() + "/role", `{"role_id":"` + roleID.String() + `"}`},
		{"delete member", http.MethodDelete, "/api/v1/members/" + memberID.String(), ""},
		{"get tenant settings", http.MethodGet, "/api/v1/tenant/settings", ""},
		{"update tenant settings", http.MethodPatch, "/api/v1/tenant/settings", `{"name":"Hijacked"}`},
		{"get tenants settings alias", http.MethodGet, "/api/v1/tenants/settings", ""},
		{"update tenants settings alias", http.MethodPatch, "/api/v1/tenants/settings", `{"name":"Hijacked"}`},
		{"get tenant location", http.MethodGet, "/api/v1/tenant/settings/location", ""},
		{"update tenant location", http.MethodPut, "/api/v1/tenant/settings/location", `{"address":"Hijacked"}`},
		{"list roles", http.MethodGet, "/api/v1/roles", ""},
		{"create role", http.MethodPost, "/api/v1/roles", `{"name":"Staff","permission_ids":["` + uuid.New().String() + `"]}`},
		{"update role", http.MethodPut, "/api/v1/roles/" + roleID.String(), `{"name":"Staff","permission_ids":["` + uuid.New().String() + `"]}`},
		{"delete role", http.MethodDelete, "/api/v1/roles/" + roleID.String(), ""},
	}
}

// TestParentTokenCannotUseForeignTenantHeader is the direct regression for the
// reported vulnerability: a parent token plus an arbitrary X-Tenant-ID must be
// refused with 403 and must never reach a use case.
func TestParentTokenCannotUseForeignTenantHeader(t *testing.T) {
	member := &recordingMemberUsecase{}
	role := &recordingRoleUsecase{}
	tenant := &recordingTenantUsecase{}
	router := newTenantContextRouter(member, role, tenant)

	parentToken := signTenantToken(t, uuid.Nil, true)
	targetRoleID, targetMemberID := uuid.New(), uuid.New()

	for _, endpoint := range tenantEndpoints(targetRoleID, targetMemberID) {
		t.Run(endpoint.name, func(t *testing.T) {
			recorder := tenantRequest(router, endpoint.method, endpoint.path, parentToken, endpoint.body, &foreignTenantHeader)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s, want 403", recorder.Code, recorder.Body.String())
			}
		})
	}

	if member.calls != 0 || role.calls != 0 || tenant.calls != 0 {
		t.Fatalf("use case reached: member=%d role=%d tenant=%d, want 0/0/0", member.calls, role.calls, tenant.calls)
	}
	if member.tenant == foreignTenantHeader || role.tenant == foreignTenantHeader || tenant.tenant == foreignTenantHeader {
		t.Fatalf("foreign tenant was propagated: member=%s role=%s tenant=%s", member.tenant, role.tenant, tenant.tenant)
	}
}

// TestParentTokenWithoutHeaderIsForbidden proves the rejection is not caused by
// the header being present; a tenantless token is refused on its own.
func TestParentTokenWithoutHeaderIsForbidden(t *testing.T) {
	member := &recordingMemberUsecase{}
	role := &recordingRoleUsecase{}
	tenant := &recordingTenantUsecase{}
	router := newTenantContextRouter(member, role, tenant)

	parentToken := signTenantToken(t, uuid.Nil, true)
	targetRoleID, targetMemberID := uuid.New(), uuid.New()

	for _, endpoint := range tenantEndpoints(targetRoleID, targetMemberID) {
		t.Run(endpoint.name, func(t *testing.T) {
			recorder := tenantRequest(router, endpoint.method, endpoint.path, parentToken, endpoint.body, nil)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s, want 403", recorder.Code, recorder.Body.String())
			}
		})
	}

	if member.calls != 0 || role.calls != 0 || tenant.calls != 0 {
		t.Fatalf("use case reached: member=%d role=%d tenant=%d, want 0/0/0", member.calls, role.calls, tenant.calls)
	}
}

// TestMemberTokenIgnoresHeaderInFavourOfClaim keeps the positive path intact:
// a member with a valid tenant claim still works, and the tenant used is the
// claim even when the header names a different tenant.
func TestMemberTokenIgnoresHeaderInFavourOfClaim(t *testing.T) {
	ownTenant := uuid.New()
	member := &recordingMemberUsecase{}
	role := &recordingRoleUsecase{}
	tenant := &recordingTenantUsecase{}
	router := newTenantContextRouter(member, role, tenant)

	memberToken := signTenantToken(t, ownTenant, false)

	recorder := tenantRequest(router, http.MethodGet, "/api/v1/members", memberToken, "", &foreignTenantHeader)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list members status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	if member.tenant != ownTenant {
		t.Fatalf("member tenant=%s, want claim tenant %s", member.tenant, ownTenant)
	}

	recorder = tenantRequest(router, http.MethodGet, "/api/v1/tenant/settings", memberToken, "", &foreignTenantHeader)
	if recorder.Code != http.StatusOK {
		t.Fatalf("tenant settings status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	if tenant.tenant != ownTenant {
		t.Fatalf("tenant settings tenant=%s, want claim tenant %s", tenant.tenant, ownTenant)
	}

	recorder = tenantRequest(router, http.MethodGet, "/api/v1/roles", memberToken, "", &foreignTenantHeader)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list roles status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	if role.tenant != ownTenant {
		t.Fatalf("roles tenant=%s, want claim tenant %s", role.tenant, ownTenant)
	}
}

// TestMemberTokenWithOwnTenantHeaderIsStillClaimBound covers the "header equals
// the caller's own tenant" edge case: the claim keeps winning, so a later
// desync between header and token cannot change the tenant in use.
func TestMemberTokenWithOwnTenantHeaderIsStillClaimBound(t *testing.T) {
	ownTenant := uuid.New()
	member := &recordingMemberUsecase{}
	router := newTenantContextRouter(member, &recordingRoleUsecase{}, &recordingTenantUsecase{})

	memberToken := signTenantToken(t, ownTenant, false)
	recorder := tenantRequest(router, http.MethodGet, "/api/v1/members", memberToken, "", &ownTenant)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", recorder.Code, recorder.Body.String())
	}
	if member.tenant != ownTenant {
		t.Fatalf("tenant=%s, want %s", member.tenant, ownTenant)
	}
}

// TestMalformedTenantHeaderIsIgnoredEntirely proves a malformed header can no
// longer turn into a 400 that reveals header parsing behaviour.
func TestMalformedTenantHeaderIsIgnoredEntirely(t *testing.T) {
	member := &recordingMemberUsecase{}
	router := newTenantContextRouter(member, &recordingRoleUsecase{}, &recordingTenantUsecase{})

	parentToken := signTenantToken(t, uuid.Nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/members", nil)
	request.Header.Set("Authorization", "Bearer "+parentToken)
	request.Header.Set("X-Tenant-ID", "not-a-uuid")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s, want 403", recorder.Code, recorder.Body.String())
	}
	if member.calls != 0 {
		t.Fatalf("member use case calls=%d, want 0", member.calls)
	}
}

// TestTenantEndpointsRequireAuthentication pins the 401 boundary so the new 403
// cannot be confused with a missing or invalid token.
func TestTenantEndpointsRequireAuthentication(t *testing.T) {
	router := newTenantContextRouter(&recordingMemberUsecase{}, &recordingRoleUsecase{}, &recordingTenantUsecase{})

	recorder := tenantRequest(router, http.MethodGet, "/api/v1/members", "", "", &foreignTenantHeader)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", recorder.Code)
	}
}
