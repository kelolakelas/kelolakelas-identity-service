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
	if roleID == uuid.Nil || tenantID == uuid.Nil {
		return domain.ErrPermissionDenied
	}

	allowed, err := checker.HasPermission(ctx, tenantID, roleID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrPermissionDenied
	}
	return nil
}
