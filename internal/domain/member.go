package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrMemberNotFound         = errors.New("member not found")
	ErrMemberRoleForbidden    = errors.New("member role cannot be changed")
	ErrMemberRoleConflict     = errors.New("role does not belong to tenant")
	ErrMemberSelfRoleChange   = errors.New("caller cannot change own member role")
	ErrMemberPermission       = errors.New("member role update permission required")
	ErrMemberDeletePermission = errors.New("member delete permission required")
)

type MemberQuery struct {
	Page     int
	PageSize int
	Search   string
	Status   string
	RoleID   *uuid.UUID
	Sort     string
	Order    string
}

type MemberRoleResponse struct {
	ID           uuid.UUID            `json:"id"`
	Name         string               `json:"name"`
	IsSystemRole bool                 `json:"is_system_role"`
	Permissions  []PermissionResponse `json:"permissions"`
}

type MemberResponse struct {
	ID        uuid.UUID          `json:"id"`
	UserID    uuid.UUID          `json:"user_id"`
	TenantID  uuid.UUID          `json:"tenant_id"`
	Email     string             `json:"email"`
	FirstName string             `json:"first_name"`
	LastName  string             `json:"last_name"`
	Phone     *string            `json:"phone,omitempty"`
	Status    string             `json:"status"`
	Role      MemberRoleResponse `json:"role"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

type MemberListResponse struct {
	Items      []MemberResponse `json:"items"`
	Pagination Pagination       `json:"pagination"`
}

type TutorQuery struct {
	Page     int
	PageSize int
	Search   string
	Status   string
}

type TutorResponse struct {
	ID        uuid.UUID `json:"id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
}

type TutorListResponse struct {
	Items      []TutorResponse `json:"items"`
	Pagination Pagination      `json:"pagination"`
}

type UpdateMemberRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

type MemberRepository interface {
	List(ctx context.Context, tenantID uuid.UUID, query MemberQuery) ([]MemberResponse, int64, error)
	GetByID(ctx context.Context, tenantID, memberID uuid.UUID) (*MemberResponse, error)
	// UpdateRole moves the tenant's member to roleID on behalf of actor. It rejects a target
	// membership that belongs to actor (ErrMemberSelfRoleChange, KEL-79) before any write.
	UpdateRole(ctx context.Context, tenantID, memberID, roleID uuid.UUID, actor Caller) (*MemberResponse, error)
	Delete(ctx context.Context, tenantID, memberID uuid.UUID) error
	// HasActiveMemberPermission grants a permission only through an active membership in the
	// tenant that currently carries the role (KEL-76).
	HasActiveMemberPermission(ctx context.Context, query MemberPermissionQuery) (bool, error)
	ListTutors(ctx context.Context, tenantID uuid.UUID, query TutorQuery) ([]TutorResponse, int64, error)
}

type MemberUsecase interface {
	List(ctx context.Context, tenantID uuid.UUID, query MemberQuery) (*MemberListResponse, error)
	GetByID(ctx context.Context, tenantID, memberID uuid.UUID) (*MemberResponse, error)
	UpdateRole(ctx context.Context, tenantID, callerRoleID, memberID, roleID uuid.UUID) (*MemberResponse, error)
	Delete(ctx context.Context, tenantID, callerRoleID, memberID uuid.UUID) error
	ListTutors(ctx context.Context, tenantID uuid.UUID, query TutorQuery) (*TutorListResponse, error)
}
