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
	ErrMemberSelfRemoval      = errors.New("caller cannot remove own membership")
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

// ActiveMembership is the caller's own membership row joined with the role it
// currently carries. KEL-136 reads it to describe the caller in a tenant without
// trusting the token claims: a membership that was removed, deactivated, or
// moved to another role must stop being describable, exactly as it stops
// authorizing anything (KEL-76).
type ActiveMembership struct {
	MemberID        uuid.UUID `json:"member_id"`
	RoleID          uuid.UUID `json:"role_id"`
	RoleName        string    `json:"role_name"`
	PermissionNames []string  `json:"permissions"`
}

type MemberRepository interface {
	List(ctx context.Context, tenantID uuid.UUID, query MemberQuery) ([]MemberResponse, int64, error)
	GetByID(ctx context.Context, tenantID, memberID uuid.UUID) (*MemberResponse, error)
	// UpdateRole moves the tenant's member to roleID on behalf of actor. It rejects a target
	// membership that belongs to actor (ErrMemberSelfRoleChange, KEL-79) before any write.
	UpdateRole(ctx context.Context, tenantID, memberID, roleID uuid.UUID, actor Caller) (*MemberResponse, error)
	// Delete soft-deletes the tenant's member on behalf of actor. It rejects a target
	// membership that belongs to actor (ErrMemberSelfRemoval, KEL-81) before any write.
	Delete(ctx context.Context, tenantID, memberID uuid.UUID, actor Caller) error
	// HasActiveMemberPermission grants a permission only through an active membership in the
	// tenant that currently carries the role (KEL-76).
	HasActiveMemberPermission(ctx context.Context, query MemberPermissionQuery) (bool, error)
	// FindActiveMembership resolves the caller's own active membership with its role and
	// permission names (KEL-136), or ErrMembershipInactive when the caller has no active
	// membership in the tenant that still carries the token's role.
	FindActiveMembership(ctx context.Context, tenantID, roleID, memberID, userID uuid.UUID) (*ActiveMembership, error)
	ListTutors(ctx context.Context, tenantID uuid.UUID, query TutorQuery) ([]TutorResponse, int64, error)
}

type MemberUsecase interface {
	List(ctx context.Context, tenantID uuid.UUID, query MemberQuery) (*MemberListResponse, error)
	GetByID(ctx context.Context, tenantID, memberID uuid.UUID) (*MemberResponse, error)
	UpdateRole(ctx context.Context, tenantID, callerRoleID, memberID, roleID uuid.UUID) (*MemberResponse, error)
	Delete(ctx context.Context, tenantID, callerRoleID, memberID uuid.UUID) error
	ListTutors(ctx context.Context, tenantID uuid.UUID, query TutorQuery) (*TutorListResponse, error)
	// FetchMyMembership describes the caller's own active membership in tenantID: role name
	// and permission names for the tenant dashboard navigation (KEL-136). It reads the
	// verified caller from the request context, so the answer follows the live membership
	// row rather than the token claims.
	FetchMyMembership(ctx context.Context, tenantID, callerRoleID uuid.UUID) (*MyMembershipResponse, error)
}
