package domain

import (
	"context"
	"github.com/google/uuid"
)

// PlatformAdminRepository is independent of tenant roles and permissions.
type PlatformAdminRepository interface {
	IsActive(ctx context.Context, userID uuid.UUID) (bool, error)
}
