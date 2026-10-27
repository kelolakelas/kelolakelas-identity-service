package domain

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrTenantSessionForbidden = errors.New("active own tenant membership required")
var ErrTenantSessionInvalid = errors.New("invalid session")

type TenantChoice struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	MemberID   uuid.UUID `json:"member_id"`
	RoleID     uuid.UUID `json:"role_id"`
	RoleName   string    `json:"role_name"`
}

type TenantSessionRepository interface {
	// The callback runs while the user and selected membership/tenant/role are
	// locked against reset, revocation, deletion and role changes.
	WithUser(context.Context, uuid.UUID, func(*User) error) error
	WithMembership(context.Context, uuid.UUID, uuid.UUID, func(*User, TenantChoice) error) error
	List(context.Context, uuid.UUID) ([]TenantChoice, error)
}
