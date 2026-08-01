package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

type roleUsecase struct {
	rbacRepo repository.RbacRepository
}

func NewRoleUsecase(rbacRepo repository.RbacRepository) RoleUsecase {
	return &roleUsecase{rbacRepo: rbacRepo}
}

func (u *roleUsecase) FetchAllPermissions(ctx context.Context) ([]domain.PermissionResponse, error) {
	permissions, err := u.rbacRepo.GetPermissions(ctx)
	if err != nil {
		return nil, err
	}

	res := make([]domain.PermissionResponse, 0, len(permissions))
	for _, p := range permissions {
		res = append(res, domain.PermissionResponse{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
		})
	}
	return res, nil
}

func (u *roleUsecase) FetchTenantRoles(ctx context.Context, tenantID uuid.UUID) ([]domain.RoleResponse, error) {
	roles, err := u.rbacRepo.GetRolesByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	res := make([]domain.RoleResponse, 0, len(roles))
	for _, r := range roles {
		res = append(res, formatRoleResponse(r))
	}
	return res, nil
}

func (u *roleUsecase) CreateCustomRole(ctx context.Context, tenantID uuid.UUID, req *domain.CreateRoleRequest) (*domain.RoleResponse, error) {
	// Check if role name already exists in this tenant
	existing, err := u.rbacRepo.GetRoleByNameAndTenantID(ctx, req.Name, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, domain.ErrRoleNameExists
	}

	roleID := uuid.New()
	role := &domain.Role{
		ID:          roleID,
		TenantID:    &tenantID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := u.rbacRepo.CreateRoleTx(ctx, role, req.PermissionIDs); err != nil {
		return nil, err
	}

	// Fetch created role with preloaded permissions
	createdRole, err := u.rbacRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	resp := formatRoleResponse(*createdRole)
	return &resp, nil
}

func (u *roleUsecase) UpdateCustomRole(ctx context.Context, tenantID, roleID uuid.UUID, req *domain.UpdateRoleRequest) (*domain.RoleResponse, error) {
	role, err := u.rbacRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	// GUARD: System Role check
	if role.TenantID == nil {
		return nil, domain.ErrSystemRoleCannotBeModified
	}

	// GUARD: IDOR check (role belongs to requesting tenant)
	if *role.TenantID != tenantID {
		return nil, domain.ErrForbiddenRoleAccess
	}

	// Check if name changed and if new name conflicts with existing role in same tenant
	if role.Name != req.Name {
		existing, err := u.rbacRepo.GetRoleByNameAndTenantID(ctx, req.Name, tenantID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, domain.ErrRoleNameExists
		}
	}

	if err := u.rbacRepo.UpdateRoleTx(ctx, roleID, req.Name, req.Description, req.PermissionIDs); err != nil {
		return nil, err
	}

	updatedRole, err := u.rbacRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}

	resp := formatRoleResponse(*updatedRole)
	return &resp, nil
}

func (u *roleUsecase) DeleteCustomRole(ctx context.Context, tenantID, roleID uuid.UUID) error {
	role, err := u.rbacRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		return err
	}

	// GUARD: System Role check
	if role.TenantID == nil {
		return domain.ErrSystemRoleCannotBeModified
	}

	// GUARD: IDOR check
	if *role.TenantID != tenantID {
		return domain.ErrForbiddenRoleAccess
	}

	// GUARD: Active members check
	count, err := u.rbacRepo.CountMembersByRole(ctx, roleID)
	if err != nil {
		return err
	}
	if count > 0 {
		return domain.ErrRoleAssignedToActiveMembers
	}

	return u.rbacRepo.DeleteRole(ctx, roleID)
}

func formatRoleResponse(role domain.Role) domain.RoleResponse {
	perms := make([]domain.PermissionResponse, 0, len(role.Permissions))
	for _, p := range role.Permissions {
		perms = append(perms, domain.PermissionResponse{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
		})
	}

	return domain.RoleResponse{
		ID:           role.ID,
		TenantID:     role.TenantID,
		Name:         role.Name,
		Description:  role.Description,
		IsSystemRole: role.TenantID == nil,
		Permissions:  perms,
	}
}
