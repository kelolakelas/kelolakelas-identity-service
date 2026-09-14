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
}

func (s *permissionCheckerStub) HasPermission(_ context.Context, _ uuid.UUID, permission string) (bool, error) {
	s.permissions = append(s.permissions, permission)
	return s.allowed, s.err
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
				_, err := NewInvitationUsecase(nil, nil, nil, checker, nil).CreateInvitation(context.Background(), tenantID, roleID, targetRoleID, "member@example.com")
				return err
			},
		},
		{
			name:       "tenant settings update",
			permission: "tenant:update",
			execute: func(checker PermissionChecker) error {
				_, err := NewTenantUsecase(nil, nil, checker, nil, nil, nil).UpdateTenantSettings(context.Background(), tenantID, roleID, &domain.UpdateTenantSettingsRequest{Name: "Tenant"})
				return err
			},
		},
		{
			name:       "tenant location update",
			permission: "tenant:update",
			execute: func(checker PermissionChecker) error {
				_, err := NewTenantUsecase(nil, nil, checker, nil, nil, nil).UpdateTenantLocation(context.Background(), tenantID, roleID, &domain.UpdateTenantLocationRequest{Address: "Jakarta"})
				return err
			},
		},
		{
			name:       "role creation",
			permission: "role:create",
			execute: func(checker PermissionChecker) error {
				_, err := NewRoleUsecase(nil, checker).CreateCustomRole(context.Background(), tenantID, roleID, &domain.CreateRoleRequest{Name: "Staff"})
				return err
			},
		},
		{
			name:       "role update",
			permission: "role:update",
			execute: func(checker PermissionChecker) error {
				_, err := NewRoleUsecase(nil, checker).UpdateCustomRole(context.Background(), tenantID, roleID, targetRoleID, &domain.UpdateRoleRequest{Name: "Staff"})
				return err
			},
		},
		{
			name:       "role deletion",
			permission: "role:delete",
			execute: func(checker PermissionChecker) error {
				return NewRoleUsecase(nil, checker).DeleteCustomRole(context.Background(), tenantID, roleID, targetRoleID)
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
		})
	}
}

func TestRequirePermissionRejectsMissingRoleWithoutLookup(t *testing.T) {
	checker := &permissionCheckerStub{allowed: true}
	err := requirePermission(context.Background(), checker, uuid.Nil, "tenant:update")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("error = %v, want permission denied", err)
	}
	if len(checker.permissions) != 0 {
		t.Fatalf("permission lookups = %v, want none", checker.permissions)
	}
}

func TestRequirePermissionAllowsAssignedPermission(t *testing.T) {
	checker := &permissionCheckerStub{allowed: true}
	if err := requirePermission(context.Background(), checker, uuid.New(), "tenant:update"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
