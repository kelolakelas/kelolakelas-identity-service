package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type rbacRepository struct {
	db *gorm.DB
}

func NewRbacRepository(db *gorm.DB) RbacRepository {
	return &rbacRepository{db: db}
}

func (r *rbacRepository) GetPermissions(ctx context.Context) ([]domain.Permission, error) {
	var permissions []domain.Permission
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&permissions).Error; err != nil {
		return nil, err
	}
	return permissions, nil
}

func (r *rbacRepository) GetRolesByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.Role, error) {
	var roles []domain.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("tenant_id = ? OR tenant_id IS NULL", tenantID).
		Order("created_at ASC").
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *rbacRepository) GetRoleByID(ctx context.Context, id uuid.UUID) (*domain.Role, error) {
	var role domain.Role
	err := r.db.WithContext(ctx).
		Preload("Permissions").
		Where("id = ?", id).
		First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrRoleNotFound
		}
		return nil, err
	}
	return &role, nil
}

func (r *rbacRepository) GetRoleByNameAndTenantID(ctx context.Context, name string, tenantID uuid.UUID) (*domain.Role, error) {
	var role domain.Role
	err := r.db.WithContext(ctx).
		Where("name = ? AND tenant_id = ?", name, tenantID).
		First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (r *rbacRepository) CreateRoleTx(ctx context.Context, role *domain.Role, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return err
		}

		for _, permID := range permissionIDs {
			rp := domain.RolePermission{
				RoleID:       role.ID,
				PermissionID: permID,
				AssignedAt:   time.Now(),
			}
			if err := tx.Create(&rp).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *rbacRepository) UpdateRoleTx(ctx context.Context, roleID uuid.UUID, name, description string, permissionIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Update Role fields
		if err := tx.Model(&domain.Role{}).Where("id = ?", roleID).Updates(map[string]interface{}{
			"name":        name,
			"description": description,
			"updated_at":  time.Now(),
		}).Error; err != nil {
			return err
		}

		// Delete existing role_permissions
		if err := tx.Where("role_id = ?", roleID).Delete(&domain.RolePermission{}).Error; err != nil {
			return err
		}

		// Insert new permission_ids
		for _, permID := range permissionIDs {
			rp := domain.RolePermission{
				RoleID:       roleID,
				PermissionID: permID,
				AssignedAt:   time.Now(),
			}
			if err := tx.Create(&rp).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *rbacRepository) DeleteRole(ctx context.Context, roleID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", roleID).Delete(&domain.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", roleID).Delete(&domain.Role{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (r *rbacRepository) CountMembersByRole(ctx context.Context, roleID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&domain.TenantMember{}).
		Where("role_id = ? AND is_active = ?", roleID, true).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}
