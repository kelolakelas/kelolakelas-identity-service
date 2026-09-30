package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// TestMemberUsecaseFetchMyMembership covers the KEL-136 membership read: the
// answer comes from the repository through the caller's own active membership,
// with the tenant and the token's role passed through, and the permission list
// is never nil so JSON keeps rendering `[]` for a role with no permissions.
func TestMemberUsecaseFetchMyMembership(t *testing.T) {
	tenantID, roleID, memberID := uuid.New(), uuid.New(), uuid.New()

	t.Run("returns the role and permission names of the active membership", func(t *testing.T) {
		stub := &memberRepositoryStub{membership: &domain.ActiveMembership{
			MemberID: memberID, RoleID: roleID, RoleName: "Creator",
			PermissionNames: []string{"member:read", "role:read", "tenant:read"},
		}}
		result, err := NewMemberUsecase(stub).FetchMyMembership(callerContext(), tenantID, roleID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stub.membershipArgs != [4]uuid.UUID{tenantID, roleID, testCaller.MemberID, testCaller.UserID} {
			t.Fatalf("repository args = %v, want tenant/role/member/user of the caller", stub.membershipArgs)
		}
		if result.RoleName != "Creator" || result.MemberID != memberID || result.RoleID != roleID {
			t.Fatalf("membership = %+v", result)
		}
		if len(result.Permissions) != 3 || result.Permissions[0] != "member:read" {
			t.Fatalf("permissions = %v", result.Permissions)
		}
	})

	t.Run("renders an empty permission list for a custom role without permissions", func(t *testing.T) {
		stub := &memberRepositoryStub{membership: &domain.ActiveMembership{
			MemberID: memberID, RoleID: roleID, RoleName: "Staff", PermissionNames: []string{},
		}}
		result, err := NewMemberUsecase(stub).FetchMyMembership(callerContext(), tenantID, roleID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Permissions == nil || len(result.Permissions) != 0 {
			t.Fatalf("permissions = %#v, want non-nil empty", result.Permissions)
		}
	})

	t.Run("returns ErrMembershipInactive when the membership is not active", func(t *testing.T) {
		stub := &memberRepositoryStub{}
		if _, err := NewMemberUsecase(stub).FetchMyMembership(callerContext(), tenantID, roleID); !errors.Is(err, domain.ErrMembershipInactive) {
			t.Fatalf("error = %v, want ErrMembershipInactive", err)
		}
	})

	t.Run("denies without a verified caller and never reaches the repository", func(t *testing.T) {
		stub := &memberRepositoryStub{membership: &domain.ActiveMembership{MemberID: memberID, RoleID: roleID, RoleName: "Creator"}}
		if _, err := NewMemberUsecase(stub).FetchMyMembership(context.Background(), tenantID, roleID); !errors.Is(err, domain.ErrMembershipInactive) {
			t.Fatalf("error = %v, want ErrMembershipInactive", err)
		}
		if stub.membershipArgs != [4]uuid.UUID{} {
			t.Fatalf("repository reached without a caller: %v", stub.membershipArgs)
		}
	})

	t.Run("denies without a token role claim and never reaches the repository", func(t *testing.T) {
		stub := &memberRepositoryStub{membership: &domain.ActiveMembership{MemberID: memberID, RoleID: roleID, RoleName: "Creator"}}
		if _, err := NewMemberUsecase(stub).FetchMyMembership(callerContext(), tenantID, uuid.Nil); !errors.Is(err, domain.ErrMembershipInactive) {
			t.Fatalf("error = %v, want ErrMembershipInactive", err)
		}
		if stub.membershipArgs != [4]uuid.UUID{} {
			t.Fatalf("repository reached without a role claim: %v", stub.membershipArgs)
		}
	})

	t.Run("returns the repository error unchanged", func(t *testing.T) {
		stub := &memberRepositoryStub{membershipErr: context.DeadlineExceeded}
		if _, err := NewMemberUsecase(stub).FetchMyMembership(callerContext(), tenantID, roleID); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want the repository error", err)
		}
	})
}
