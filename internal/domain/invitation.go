package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvitationAccountMismatch = errors.New("invitation belongs to another account")
	ErrExistingTeacherRequired   = errors.New("existing invitations require an active Teacher account and Teacher role")
	ErrInvitationMemberInactive  = errors.New("existing tenant membership is inactive or deleted")
	ErrInvitationNotFound        = errors.New("invitation not found")
	ErrInvitationExpired         = errors.New("invitation token has expired")
	ErrInvitationUsed            = errors.New("invitation token has already been used")
	ErrAlreadyTenantMember       = errors.New("user is already a member of this tenant")
	// ErrInvitationRoleInvalid rejects invitations that would hand out a role owned by
	// another tenant. The invited role must belong to the target tenant or be a system role.
	ErrInvitationRoleInvalid = errors.New("role does not belong to tenant or is not a system role")
)

type TenantInvitation struct {
	AcceptedMemberID *uuid.UUID `gorm:"type:uuid" json:"-"`
	TenantName       string     `gorm:"-" json:"tenant_name,omitempty"`
	RoleName         string     `gorm:"-" json:"role_name,omitempty"`
	ExistingAccount  bool       `gorm:"-" json:"existing_account"`
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	RoleID           uuid.UUID  `gorm:"type:uuid;not null" json:"role_id"`
	Email            string     `gorm:"type:varchar(255);not null" json:"email"`
	Token            string     `gorm:"type:varchar(255);unique;not null" json:"token"`
	IsUsed           bool       `gorm:"type:boolean;default:false" json:"is_used"`
	ExpiresAt        time.Time  `gorm:"type:timestamp;not null" json:"expires_at"`
	CreatedAt        time.Time  `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	UpdatedAt        time.Time  `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`

	// EmailSent reports whether the invitation email was delivered when the
	// invitation was created. It is a transient response field: the delivery
	// attempt is not part of the stored invitation state (KEL-36), so it is
	// excluded from persistence and only populated in memory.
	EmailSent bool `gorm:"-" json:"email_sent"`
}

func (TenantInvitation) TableName() string {
	return "tenant_invitations"
}
