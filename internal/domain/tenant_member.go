package domain

import (
	"time"

	"github.com/google/uuid"
)

type TenantMember struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_user" json:"tenant_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_tenant_user" json:"user_id"`
	RoleID    uuid.UUID `gorm:"type:uuid;not null" json:"role_id"`
	IsActive  bool      `gorm:"type:boolean;default:true" json:"is_active"`
	JoinedAt  time.Time `gorm:"type:timestamp;not null;default:now()" json:"joined_at"`
	UpdatedAt time.Time `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
}

func (TenantMember) TableName() string {
	return "tenant_members"
}
