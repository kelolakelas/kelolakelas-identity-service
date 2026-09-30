package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// FindActiveMembership resolves the membership the caller describes themselves
// with, in one read.
//
// The membership must be active (is_active, deleted_at IS NULL), belong to
// tenantID, and currently carry roleID; the role must belong to that tenant or
// be a system role (tenant_id IS NULL), preserving the ADR 0010 tenant scope.
// A token carrying member_id is pinned to that exact membership row, and the
// user id must also match, so a token from a removed membership never describes
// a later re-invitation. An unknown or inactive membership answers
// ErrMembershipInactive rather than a distinguished error, so a caller cannot
// probe which user ids hold memberships.
func FindActiveMembership(ctx context.Context, db *gorm.DB, tenantID, roleID, memberID, userID uuid.UUID) (*domain.ActiveMembership, error) {
	// tenant_members is queried through a raw table name, so GORM's soft-delete
	// scope does not apply and "tm.deleted_at IS NULL" must stay explicit.
	query := db.WithContext(ctx).
		Table("tenant_members tm").
		Select("tm.tenant_id, tm.id AS member_id, tm.user_id, tm.role_id, ro.name AS role_name").
		Joins("JOIN roles ro ON ro.id = tm.role_id").
		Where("tm.tenant_id = ? AND tm.role_id = ? AND tm.is_active = ? AND tm.deleted_at IS NULL", tenantID, roleID, true).
		Where("(ro.tenant_id = ? OR ro.tenant_id IS NULL)", tenantID)
	if memberID != uuid.Nil {
		query = query.Where("tm.id = ?", memberID)
	}
	if userID != uuid.Nil {
		query = query.Where("tm.user_id = ?", userID)
	}

	var membership domain.ActiveMembership
	// Scan into a flat row first: the domain struct carries a []string the SQL
	// result cannot populate, which GORM would log as a parse error on every call.
	var row struct {
		TenantID uuid.UUID `gorm:"column:tenant_id"`
		MemberID uuid.UUID `gorm:"column:member_id"`
		UserID   uuid.UUID `gorm:"column:user_id"`
		RoleID   uuid.UUID `gorm:"column:role_id"`
		RoleName string    `gorm:"column:role_name"`
	}
	if err := query.Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.MemberID == uuid.Nil {
		return nil, domain.ErrMembershipInactive
	}
	membership = domain.ActiveMembership{MemberID: row.MemberID, RoleID: row.RoleID, RoleName: row.RoleName}

	var permissions []struct {
		Name string `gorm:"column:name"`
	}
	if err := db.WithContext(ctx).
		Table("role_permissions rp").
		Select("p.name").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Where("rp.role_id = ?", membership.RoleID).
		Order("p.name ASC").
		Scan(&permissions).Error; err != nil {
		return nil, err
	}
	membership.PermissionNames = make([]string, 0, len(permissions))
	for _, p := range permissions {
		membership.PermissionNames = append(membership.PermissionNames, p.Name)
	}
	return &membership, nil
}
