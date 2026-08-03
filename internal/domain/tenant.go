package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrTenantNameAlreadyExists = errors.New("tenant name already exists")
)

type Tenant struct {
	ID               uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name             string           `gorm:"type:varchar(255);unique;not null" json:"name"`
	Phone            *string          `gorm:"type:varchar(50)" json:"phone,omitempty"`
	Address          *string          `gorm:"type:text" json:"address,omitempty"`
	About            *json.RawMessage `gorm:"type:jsonb;serializer:json" json:"about,omitempty"`
	PaymentAccountID *string          `gorm:"type:varchar(255);unique" json:"payment_account_id,omitempty"`
	Status           string           `gorm:"type:varchar(50);default:'active'" json:"status"`
	CreatedAt        time.Time        `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	UpdatedAt        time.Time        `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
	DeletedAt        gorm.DeletedAt   `gorm:"index" json:"deleted_at,omitempty"`
}

type RegisterTenantRequest struct {
	Email         string  `json:"email" binding:"required,email"`
	Password      string  `json:"password" binding:"required,min=6"`
	FirstName     string  `json:"first_name" binding:"required"`
	LastName      string  `json:"last_name" binding:"required"`
	Phone         *string `json:"phone,omitempty"`
	TenantName    string  `json:"tenant_name" binding:"required"`
	TenantPhone   *string `json:"tenant_phone,omitempty"`
	TenantAddress *string `json:"tenant_address,omitempty"`
}

type RegisterTenantResponse struct {
	Token    string    `json:"token"`
	User     User      `json:"user"`
	Tenant   Tenant    `json:"tenant"`
	TenantID uuid.UUID `json:"tenant_id"`
}

type UpdateTenantSettingsRequest struct {
	Name    string           `json:"name" binding:"required"`
	Phone   *string          `json:"phone,omitempty"`
	Address *string          `json:"address,omitempty"`
	About   *json.RawMessage `json:"about,omitempty"`
}

type TenantUsecase interface {
	RegisterTenant(ctx context.Context, req *RegisterTenantRequest) (*RegisterTenantResponse, error)
	GetTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
	UpdateTenantSettings(ctx context.Context, id uuid.UUID, req *UpdateTenantSettingsRequest) (*Tenant, error)
}
