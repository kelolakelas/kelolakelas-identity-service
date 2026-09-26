package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// ActiveMemberHasPermission answers a domain.MemberPermissionQuery with a single count query.
// It is shared by the HTTP permission checks and the gRPC CheckPermission member_id path so
// both enforce the same membership rule.
//
// The membership must be active (is_active, deleted_at IS NULL), belong to the tenant, and
// currently carry the role. The role must belong to the tenant or be a system role
// (tenant_id IS NULL, e.g. Creator or Teacher), preserving the ADR 0010 tenant scope. An
// incomplete query is denied without touching the database, and a database error is returned
// to the caller, never treated as allowed.
func ActiveMemberHasPermission(ctx context.Context, db *gorm.DB, q domain.MemberPermissionQuery) (bool, error) {
	if !q.Complete() {
		return false, nil
	}

	// tenant_members is queried through a raw table name, so GORM's soft-delete scope does
	// not apply and "tm.deleted_at IS NULL" must stay explicit.
	query := db.WithContext(ctx).
		Table("tenant_members tm").
		Joins("JOIN roles ro ON ro.id = tm.role_id").
		Joins("JOIN role_permissions rp ON rp.role_id = ro.id").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Where("tm.tenant_id = ? AND tm.role_id = ? AND tm.is_active = ? AND tm.deleted_at IS NULL", q.TenantID, q.RoleID, true).
		Where("p.name = ?", q.Permission).
		Where("(ro.tenant_id = ? OR ro.tenant_id IS NULL)", q.TenantID)
	if q.MemberID != uuid.Nil {
		query = query.Where("tm.id = ?", q.MemberID)
	}
	if q.UserID != uuid.Nil {
		query = query.Where("tm.user_id = ?", q.UserID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
