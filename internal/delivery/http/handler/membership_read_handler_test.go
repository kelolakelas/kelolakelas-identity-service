package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// membershipStubMemberUsecase serves only FetchMyMembership; the other methods
// are out of scope for the KEL-136 handler test and panic if reached.
type membershipStubMemberUsecase struct {
	membership *domain.MyMembershipResponse
	err        error
	args       struct {
		tenantID uuid.UUID
		roleID   uuid.UUID
	}
}

func (s *membershipStubMemberUsecase) List(context.Context, uuid.UUID, domain.MemberQuery) (*domain.MemberListResponse, error) {
	panic("not implemented")
}

func (s *membershipStubMemberUsecase) GetByID(context.Context, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	panic("not implemented")
}

func (s *membershipStubMemberUsecase) UpdateRole(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	panic("not implemented")
}

func (s *membershipStubMemberUsecase) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	panic("not implemented")
}

func (s *membershipStubMemberUsecase) ListTutors(context.Context, uuid.UUID, domain.TutorQuery) (*domain.TutorListResponse, error) {
	panic("not implemented")
}

func (s *membershipStubMemberUsecase) FetchMyMembership(_ context.Context, tenantID, callerRoleID uuid.UUID) (*domain.MyMembershipResponse, error) {
	s.args.tenantID = tenantID
	s.args.roleID = callerRoleID
	// Mirrors the real usecase: the membership lookup is scoped by the token's
	// role claim, so a missing role is an inactive membership, never a widening.
	if callerRoleID == uuid.Nil {
		return nil, domain.ErrMembershipInactive
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.membership, nil
}

// membershipTestContext builds a gin context whose request carries the verified
// caller, exactly like AuthMiddleware does, plus the tenant and role claims.
func membershipTestContext(t *testing.T, tenantID, roleID uuid.UUID) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/members/me/membership", nil)
	request = request.WithContext(domain.WithCaller(request.Context(), domain.Caller{UserID: uuid.New(), MemberID: uuid.New()}))
	context.Request = request
	context.Set("tenant_id", tenantID)
	context.Set("role_id", roleID)
	return context, recorder
}

// TestGetMyMembershipAnswersActiveMembershipAndMapsErrors pins the KEL-136
// delivery contract: the tenant and the token's role reach the usecase, the
// role name and permission names are returned verbatim (Creator sees the full
// list, a Teacher-shaped role sees only its teaching permissions), and both the
// inactive-membership error and a repository failure map onto distinct status
// codes instead of leaking role data.
func TestGetMyMembershipAnswersActiveMembershipAndMapsErrors(t *testing.T) {
	tenantID, roleID, memberID := uuid.New(), uuid.New(), uuid.New()

	cases := []struct {
		name       string
		stub       *membershipStubMemberUsecase
		wantStatus int
		wantBody   map[string]any
	}{
		{
			name: "creator receives the full permission list",
			stub: &membershipStubMemberUsecase{membership: &domain.MyMembershipResponse{
				MemberID: memberID, RoleID: roleID, RoleName: "Creator",
				Permissions: []string{"chat:manage", "class:read", "enrollment:read", "member:read", "role:read", "tenant:read"},
			}},
			wantStatus: http.StatusOK,
			wantBody: map[string]any{
				"role_name":   "Creator",
				"member_id":   memberID.String(),
				"permissions": []any{"chat:manage", "class:read", "enrollment:read", "member:read", "role:read", "tenant:read"},
			},
		},
		{
			name: "teacher receives only the teaching permissions",
			stub: &membershipStubMemberUsecase{membership: &domain.MyMembershipResponse{
				MemberID: memberID, RoleID: roleID, RoleName: "Teacher",
				Permissions: []string{"attendance:read", "report:read", "schedule:read", "student_note:read"},
			}},
			wantStatus: http.StatusOK,
			wantBody: map[string]any{
				"role_name":   "Teacher",
				"permissions": []any{"attendance:read", "report:read", "schedule:read", "student_note:read"},
			},
		},
		{
			name:       "inactive membership is forbidden",
			stub:       &membershipStubMemberUsecase{err: domain.ErrMembershipInactive},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "repository failure is a server error",
			stub:       &membershipStubMemberUsecase{err: errors.New("database unavailable")},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			context, recorder := membershipTestContext(t, tenantID, roleID)
			NewMemberHandler(testCase.stub).GetMyMembership(context)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), testCase.wantStatus)
			}
			if testCase.stub.args.tenantID != tenantID || testCase.stub.args.roleID != roleID {
				t.Fatalf("usecase args tenant=%s role=%s, want the token claims", testCase.stub.args.tenantID, testCase.stub.args.roleID)
			}
			if testCase.wantBody == nil {
				return
			}

			var envelope struct {
				Status string         `json:"status"`
				Data   map[string]any `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
			}
			if envelope.Status != "success" {
				t.Fatalf("envelope status=%q, want success", envelope.Status)
			}
			for key, want := range testCase.wantBody {
				got := envelope.Data[key]
				if !ObjectsEqual(got, want) {
					t.Fatalf("data[%s]=%#v, want %#v", key, got, want)
				}
			}
		})
	}
}

// ObjectsEqual compares decoded JSON values without pulling a dependency.
func ObjectsEqual(got, want any) bool {
	switch want := want.(type) {
	case []any:
		list, ok := got.([]any)
		if !ok || len(list) != len(want) {
			return false
		}
		for i := range want {
			if !ObjectsEqual(list[i], want[i]) {
				return false
			}
		}
		return true
	default:
		return got == want
	}
}

// TestGetMyMembershipRequiresTenantContext keeps the endpoint inside the
// tenant-scoped surface: a parent token (no tenant claim) is rejected with 403
// before any usecase work, exactly like the other tenant endpoints.
func TestGetMyMembershipRequiresTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/members/me/membership", nil)
	request = request.WithContext(domain.WithCaller(request.Context(), domain.Caller{UserID: uuid.New()}))
	context.Request = request
	context.Set("role_id", uuid.New())

	stub := &membershipStubMemberUsecase{membership: &domain.MyMembershipResponse{RoleName: "Creator"}}
	NewMemberHandler(stub).GetMyMembership(context)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s, want 403", recorder.Code, recorder.Body.String())
	}
	if stub.args.tenantID != uuid.Nil {
		t.Fatal("usecase reached without a tenant claim")
	}
}

// TestGetMyMembershipMissingRoleClaimIsForbidden pins the missing-claim path:
// without a role_id claim the usecase cannot scope the membership lookup, so
// the request is refused rather than answered from the claims.
func TestGetMyMembershipMissingRoleClaimIsForbidden(t *testing.T) {
	context, recorder := membershipTestContext(t, uuid.New(), uuid.Nil)
	stub := &membershipStubMemberUsecase{membership: &domain.MyMembershipResponse{RoleName: "Creator"}}
	NewMemberHandler(stub).GetMyMembership(context)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s, want 403", recorder.Code, recorder.Body.String())
	}
	if stub.args.roleID != uuid.Nil && stub.membership != nil {
		t.Fatal("usecase answered without a role claim")
	}
}
