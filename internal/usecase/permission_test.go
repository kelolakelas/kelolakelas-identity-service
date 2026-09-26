package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type permissionCheckerStub struct {
	allowed     bool
	err         error
	permissions []string
	tenants     []uuid.UUID
	queries     []domain.MemberPermissionQuery
}

func (s *permissionCheckerStub) HasActiveMemberPermission(_ context.Context, query domain.MemberPermissionQuery) (bool, error) {
	s.permissions = append(s.permissions, query.Permission)
	s.tenants = append(s.tenants, query.TenantID)
	s.queries = append(s.queries, query)
	return s.allowed, s.err
}

// testCaller is the verified caller attached by callerContext.
var testCaller = domain.Caller{
	UserID:   uuid.MustParse("11111111-1111-4111-8111-111111111111"),
	MemberID: uuid.MustParse("22222222-2222-4222-8222-222222222222"),
}

// callerContext returns the request context AuthMiddleware produces for a tenant token that
// carries the member_id claim.
func callerContext() context.Context {
	return domain.WithCaller(context.Background(), testCaller)
}

func TestAdministrativeMutationsRequirePermissionBeforeAccessingRepositories(t *testing.T) {
	tenantID := uuid.New()
	roleID := uuid.New()
	targetRoleID := uuid.New()

	tests := []struct {
		name       string
		permission string
		execute    func(PermissionChecker) error
	}{
		{
			name:       "invitation creation",
			permission: "member:invite",
			execute: func(checker PermissionChecker) error {
				_, err := NewInvitationUsecase(nil, nil, nil, nil, checker, nil).CreateInvitation(callerContext(), tenantID, roleID, targetRoleID, "member@example.com")
				return err
			},
		},
		{
			name:       "invitation listing",
			permission: "member:invite",
			execute: func(checker PermissionChecker) error {
				_, err := NewInvitationUsecase(nil, nil, nil, nil, checker, nil).ListInvitations(callerContext(), tenantID, roleID)
				return err
			},
		},
		{
			name:       "invitation revocation",
			permission: "member:invite",
			execute: func(checker PermissionChecker) error {
				return NewInvitationUsecase(nil, nil, nil, nil, checker, nil).RevokeInvitation(callerContext(), tenantID, roleID, uuid.New())
			},
		},
		{
			name:       "tenant settings update",
			permission: "tenant:update",
			execute: func(checker PermissionChecker) error {
				_, err := NewTenantUsecase(nil, nil, checker, nil, nil, nil).UpdateTenantSettings(callerContext(), tenantID, roleID, &domain.UpdateTenantSettingsRequest{Name: "Tenant"})
				return err
			},
		},
		{
			name:       "tenant location update",
			permission: "tenant:update",
			execute: func(checker PermissionChecker) error {
				_, err := NewTenantUsecase(nil, nil, checker, nil, nil, nil).UpdateTenantLocation(callerContext(), tenantID, roleID, &domain.UpdateTenantLocationRequest{Address: "Jakarta"})
				return err
			},
		},
		{
			name:       "role creation",
			permission: "role:create",
			execute: func(checker PermissionChecker) error {
				_, err := NewRoleUsecase(nil, checker).CreateCustomRole(callerContext(), tenantID, roleID, &domain.CreateRoleRequest{Name: "Staff"})
				return err
			},
		},
		{
			name:       "role update",
			permission: "role:update",
			execute: func(checker PermissionChecker) error {
				_, err := NewRoleUsecase(nil, checker).UpdateCustomRole(callerContext(), tenantID, roleID, targetRoleID, &domain.UpdateRoleRequest{Name: "Staff"})
				return err
			},
		},
		{
			name:       "role deletion",
			permission: "role:delete",
			execute: func(checker PermissionChecker) error {
				return NewRoleUsecase(nil, checker).DeleteCustomRole(callerContext(), tenantID, roleID, targetRoleID)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checker := &permissionCheckerStub{}
			err := test.execute(checker)
			if !errors.Is(err, domain.ErrPermissionDenied) {
				t.Fatalf("error = %v, want permission denied", err)
			}
			if len(checker.permissions) != 1 || checker.permissions[0] != test.permission {
				t.Fatalf("permissions = %v, want [%s]", checker.permissions, test.permission)
			}
			if len(checker.tenants) != 1 || checker.tenants[0] != tenantID {
				t.Fatalf("tenants = %v, want [%s]", checker.tenants, tenantID)
			}
			// KEL-76: every administrative check is answered for the caller's membership,
			// never for the role alone.
			want := domain.MemberPermissionQuery{TenantID: tenantID, RoleID: roleID, MemberID: testCaller.MemberID, UserID: testCaller.UserID, Permission: test.permission}
			if checker.queries[0] != want {
				t.Fatalf("query = %+v, want %+v", checker.queries[0], want)
			}
		})
	}
}

func TestRequirePermissionRejectsMissingRoleWithoutLookup(t *testing.T) {
	checker := &permissionCheckerStub{allowed: true}
	err := requirePermission(callerContext(), checker, uuid.New(), uuid.Nil, "tenant:update")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("error = %v, want permission denied", err)
	}
	if len(checker.permissions) != 0 {
		t.Fatalf("permission lookups = %v, want none", checker.permissions)
	}
}

func TestRequirePermissionRejectsMissingTenantWithoutLookup(t *testing.T) {
	checker := &permissionCheckerStub{allowed: true}
	err := requirePermission(callerContext(), checker, uuid.Nil, uuid.New(), "tenant:update")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("error = %v, want permission denied", err)
	}
	if len(checker.permissions) != 0 {
		t.Fatalf("permission lookups = %v, want none", checker.permissions)
	}
}

// TestRequirePermissionRejectsMissingCallerWithoutLookup proves the KEL-76 fail-closed rule: a
// request whose context carries no verified caller (or a caller without a user id) is denied
// without asking the database, even when the role would grant the permission.
func TestRequirePermissionRejectsMissingCallerWithoutLookup(t *testing.T) {
	contexts := map[string]context.Context{
		"no caller":           context.Background(),
		"caller without user": domain.WithCaller(context.Background(), domain.Caller{MemberID: uuid.New()}),
	}
	for name, ctx := range contexts {
		t.Run(name, func(t *testing.T) {
			checker := &permissionCheckerStub{allowed: true}
			err := requirePermission(ctx, checker, uuid.New(), uuid.New(), "tenant:update")
			if !errors.Is(err, domain.ErrPermissionDenied) {
				t.Fatalf("error = %v, want permission denied", err)
			}
			if len(checker.permissions) != 0 {
				t.Fatalf("permission lookups = %v, want none", checker.permissions)
			}
		})
	}
}

func TestRequirePermissionAllowsAssignedPermission(t *testing.T) {
	tenantID := uuid.New()
	checker := &permissionCheckerStub{allowed: true}
	if err := requirePermission(callerContext(), checker, tenantID, uuid.New(), "tenant:update"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(checker.tenants) != 1 || checker.tenants[0] != tenantID {
		t.Fatalf("tenants = %v, want [%s]", checker.tenants, tenantID)
	}
}

// TestRequirePermissionLegacyTokenMatchesByUser documents the explicit rule for tokens issued
// without the member_id claim: the lookup is still made, keyed by the caller's user in the
// token tenant with the token role, so a removed or re-roled member is still rejected.
func TestRequirePermissionLegacyTokenMatchesByUser(t *testing.T) {
	tenantID, roleID, userID := uuid.New(), uuid.New(), uuid.New()
	checker := &permissionCheckerStub{allowed: true}
	ctx := domain.WithCaller(context.Background(), domain.Caller{UserID: userID})
	if err := requirePermission(ctx, checker, tenantID, roleID, "role:create"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := domain.MemberPermissionQuery{TenantID: tenantID, RoleID: roleID, UserID: userID, Permission: "role:create"}
	if len(checker.queries) != 1 || checker.queries[0] != want {
		t.Fatalf("queries = %+v, want [%+v]", checker.queries, want)
	}
}

// TestRequirePermissionFailsClosedOnLookupError proves a database failure is returned to the
// handler (which maps it to its existing 5xx response) instead of being treated as allowed.
func TestRequirePermissionFailsClosedOnLookupError(t *testing.T) {
	wantErr := errors.New("database unavailable")
	checker := &permissionCheckerStub{allowed: true, err: wantErr}
	err := requirePermission(callerContext(), checker, uuid.New(), uuid.New(), "tenant:update")
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
