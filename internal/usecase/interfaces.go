package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type UserUsecase interface {
	Register(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type TenantUsecase interface {
	CreateTenant(ctx context.Context, tenant *domain.Tenant) error
	GetTenantByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *domain.Tenant) error
}

type TenantMemberUsecase interface {
	AddMember(ctx context.Context, member *domain.TenantMember) error
	RemoveMember(ctx context.Context, id uuid.UUID) error
}

type InvitationUsecase interface {
	CreateInvitation(ctx context.Context, tenantID, roleID uuid.UUID, email string) (*domain.TenantInvitation, error)
	VerifyInvitation(ctx context.Context, token string) (*domain.TenantInvitation, error)
}

type RoleUsecase interface {
	FetchAllPermissions(ctx context.Context) ([]domain.PermissionResponse, error)
	FetchTenantRoles(ctx context.Context, tenantID uuid.UUID) ([]domain.RoleResponse, error)
	CreateCustomRole(ctx context.Context, tenantID uuid.UUID, req *domain.CreateRoleRequest) (*domain.RoleResponse, error)
	UpdateCustomRole(ctx context.Context, tenantID, roleID uuid.UUID, req *domain.UpdateRoleRequest) (*domain.RoleResponse, error)
	DeleteCustomRole(ctx context.Context, tenantID, roleID uuid.UUID) error
}
