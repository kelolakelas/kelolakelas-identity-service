package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func requirePermission(ctx context.Context, checker PermissionChecker, roleID uuid.UUID, permission string) error {
	if roleID == uuid.Nil {
		return domain.ErrPermissionDenied
	}

	allowed, err := checker.HasPermission(ctx, roleID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrPermissionDenied
	}
	return nil
}
