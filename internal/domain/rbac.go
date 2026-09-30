package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRoleNotFound                = errors.New("role not found")
	ErrRoleNameExists              = errors.New("role with this name already exists in tenant")
	ErrSystemRoleCannotBeModified  = errors.New("system roles cannot be modified")
	ErrForbiddenRoleAccess         = errors.New("forbidden: role belongs to another tenant")
	ErrRoleAssignedToActiveMembers = errors.New("cannot delete role because it is currently assigned to active members")
	ErrPermissionDenied            = errors.New("permission denied")
	ErrCreatorGrantForbidden       = errors.New("creator role requires platform approval")
	// ErrMembershipInactive reports that the caller's membership in the tenant is
	// not active (removed, deactivated, or moved off the token's role). KEL-136's
	// membership read must fail closed on that state instead of answering stale
	// role data from the token claims.
	ErrMembershipInactive = errors.New("membership is not active")
)

type Permission struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name        string    `gorm:"type:varchar(255);unique;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
}

func (Permission) TableName() string {
	return "permissions"
}

type Role struct {
	ID          uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID    *uuid.UUID   `gorm:"type:uuid" json:"tenant_id,omitempty"`
	Name        string       `gorm:"type:varchar(255);not null" json:"name"`
	Description string       `gorm:"type:text" json:"description,omitempty"`
	CreatedAt   time.Time    `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	UpdatedAt   time.Time    `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
}

func (Role) TableName() string {
	return "roles"
}

type RolePermission struct {
	RoleID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"role_id"`
	PermissionID uuid.UUID `gorm:"type:uuid;primaryKey" json:"permission_id"`
	AssignedAt   time.Time `gorm:"type:timestamp;not null;default:now()" json:"assigned_at"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

type CreateRoleRequest struct {
	Name          string      `json:"name" binding:"required"`
	Description   string      `json:"description"`
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required"`
}

type UpdateRoleRequest struct {
	Name          string      `json:"name" binding:"required"`
	Description   string      `json:"description"`
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required"`
}

type PermissionResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
}

type RoleResponse struct {
	ID           uuid.UUID            `json:"id"`
	TenantID     *uuid.UUID           `json:"tenant_id,omitempty"`
	Name         string               `json:"name"`
	Description  string               `json:"description,omitempty"`
	IsSystemRole bool                 `json:"is_system_role"`
	Permissions  []PermissionResponse `json:"permissions"`
}

// MyMembershipResponse answers "who am I in this tenant" (KEL-136). It carries the
// role name the member footer shows and the permission names the sidebar filters
// on, taken from the caller's active membership — never from the token claims,
// which can outlive the membership they were minted for.
type MyMembershipResponse struct {
	MemberID    uuid.UUID `json:"member_id"`
	RoleID      uuid.UUID `json:"role_id"`
	RoleName    string    `json:"role_name"`
	Permissions []string  `json:"permissions"`
}
