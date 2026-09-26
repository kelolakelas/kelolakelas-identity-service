package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// requirePermission authorizes an administrative mutation against the tenant the caller is
// currently operating on. Passing the tenant keeps the decision tenant-scoped: a role from
// another tenant can never satisfy the check.
func requirePermission(ctx context.Context, checker PermissionChecker, tenantID, roleID uuid.UUID, permission string) error {
	allowed, err := callerHasPermission(ctx, checker, tenantID, roleID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrPermissionDenied
	}
	return nil
}

// callerHasPermission is the single HTTP permission decision (KEL-76). Besides the tenant and
// role claims it needs the verified caller that AuthMiddleware attaches to the request
// context; the repository then grants the permission only through that caller's active
// membership in the tenant that still carries the token's role.
//
// A token with a member_id claim is pinned to that exact membership row (and its user), so a
// token from a removed membership never authorizes a later re-invitation. A token issued
// without the claim is matched by (tenant, user, role), which still rejects a removed,
// deactivated or re-roled member.
//
// A request without a tenant, role or caller user is denied without a lookup; a lookup error
// is returned to the caller, never treated as allowed.
func callerHasPermission(ctx context.Context, checker PermissionChecker, tenantID, roleID uuid.UUID, permission string) (bool, error) {
	caller, ok := domain.CallerFromContext(ctx)
	if !ok || caller.UserID == uuid.Nil {
		return false, nil
	}
	query := domain.MemberPermissionQuery{
		TenantID:   tenantID,
		RoleID:     roleID,
		MemberID:   caller.MemberID,
		UserID:     caller.UserID,
		Permission: permission,
	}
	if !query.Complete() {
		return false, nil
	}
	return checker.HasActiveMemberPermission(ctx, query)
}
