package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvitationNotFound  = errors.New("invitation not found")
	ErrInvitationExpired   = errors.New("invitation token has expired")
	ErrInvitationUsed      = errors.New("invitation token has already been used")
	ErrAlreadyTenantMember = errors.New("user is already a member of this tenant")
)

type TenantInvitation struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	RoleID    uuid.UUID `gorm:"type:uuid;not null" json:"role_id"`
	Email     string    `gorm:"type:varchar(255);not null" json:"email"`
	Token     string    `gorm:"type:varchar(255);unique;not null" json:"token"`
	IsUsed    bool      `gorm:"type:boolean;default:false" json:"is_used"`
	ExpiresAt time.Time `gorm:"type:timestamp;not null" json:"expires_at"`
	CreatedAt time.Time `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
}

func (TenantInvitation) TableName() string {
	return "tenant_invitations"
}
