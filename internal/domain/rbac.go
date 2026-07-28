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
