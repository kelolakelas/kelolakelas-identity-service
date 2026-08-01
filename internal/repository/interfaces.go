package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id uuid.UUID) error
	RegisterTenantTx(ctx context.Context, user *domain.User, tenant *domain.Tenant) (*domain.TenantMember, error)
	RegisterInvitedUserTx(ctx context.Context, token, firstName, lastName, password string) (*domain.User, error)
	GetPermissionsByRoleId(ctx context.Context, roleID uuid.UUID) ([]string, error)
	GetTenantMemberByUserID(ctx context.Context, userID uuid.UUID) (*domain.TenantMember, error)
}

type TenantRepository interface {
	Create(ctx context.Context, tenant *domain.Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	IsNameExists(ctx context.Context, name string) (bool, error)
	Update(ctx context.Context, tenant *domain.Tenant) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type TenantMemberRepository interface {
	Create(ctx context.Context, member *domain.TenantMember) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.TenantMember, error)
	GetByTenantAndUser(ctx context.Context, tenantID, userID uuid.UUID) (*domain.TenantMember, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type InvitationRepository interface {
	Create(ctx context.Context, invitation *domain.TenantInvitation) error
	GetByToken(ctx context.Context, token string) (*domain.TenantInvitation, error)
	GetByTenantAndEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.TenantInvitation, error)
	Update(ctx context.Context, invitation *domain.TenantInvitation) error
}

type RbacRepository interface {
	GetPermissions(ctx context.Context) ([]domain.Permission, error)
	GetRolesByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.Role, error)
	GetRoleByID(ctx context.Context, id uuid.UUID) (*domain.Role, error)
	GetRoleByNameAndTenantID(ctx context.Context, name string, tenantID uuid.UUID) (*domain.Role, error)
	CreateRoleTx(ctx context.Context, role *domain.Role, permissionIDs []uuid.UUID) error
	UpdateRoleTx(ctx context.Context, roleID uuid.UUID, name, description string, permissionIDs []uuid.UUID) error
	DeleteRole(ctx context.Context, roleID uuid.UUID) error
	CountMembersByRole(ctx context.Context, roleID uuid.UUID) (int64, error)
}
