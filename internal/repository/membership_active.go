package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IsActiveTenantMember reports whether memberID names a membership that
// belongs to tenantID, is active, and is not soft-deleted. Another tenant's
// member, an inactive member, and an unknown id all answer false, never an
// error, so the caller cannot distinguish them. A database failure is
// returned so the caller fails closed instead of guessing.
//
// tenant_members is queried through a raw table name, so GORM's soft-delete
// scope does not apply and "deleted_at IS NULL" must stay explicit.
func IsActiveTenantMember(ctx context.Context, db *gorm.DB, tenantID, memberID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).
		Table("tenant_members").
		Where("id = ? AND tenant_id = ? AND is_active = ? AND deleted_at IS NULL", memberID, tenantID, true).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
