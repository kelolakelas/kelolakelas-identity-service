package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrTenantNameAlreadyExists = errors.New("tenant name already exists")
)

type Tenant struct {
	ID                uuid.UUID        `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name              string           `gorm:"type:varchar(255);unique;not null" json:"name"`
	Phone             *string          `gorm:"type:varchar(50)" json:"phone,omitempty"`
	Address           *string          `gorm:"type:text" json:"address,omitempty"`
	AddressFormatted  *string          `gorm:"type:varchar(500)" json:"address_formatted,omitempty"`
	Latitude          *float64         `gorm:"type:decimal(10,7)" json:"latitude,omitempty"`
	Longitude         *float64         `gorm:"type:decimal(10,7)" json:"longitude,omitempty"`
	GooglePlaceID     *string          `gorm:"type:varchar(255)" json:"google_place_id,omitempty"`
	LocationAccuracy  *float64         `gorm:"type:decimal" json:"location_accuracy_meters,omitempty"`
	LocationUpdatedAt *time.Time       `gorm:"type:timestamp" json:"location_updated_at,omitempty"`
	About             *json.RawMessage `gorm:"type:jsonb;serializer:json" json:"about,omitempty"`
	PaymentAccountID  *string          `gorm:"type:varchar(255);unique" json:"payment_account_id,omitempty"`
	Status            string           `gorm:"type:varchar(50);default:'active'" json:"status"`
	CreatedAt         time.Time        `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	UpdatedAt         time.Time        `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
	DeletedAt         gorm.DeletedAt   `gorm:"index" json:"deleted_at,omitempty"`
}

type TenantLocation struct {
	Address                string     `json:"address"`
	AddressFormatted       *string    `json:"address_formatted,omitempty"`
	Latitude               *float64   `json:"latitude,omitempty"`
	Longitude              *float64   `json:"longitude,omitempty"`
	GooglePlaceID          *string    `json:"google_place_id,omitempty"`
	LocationAccuracyMeters *float64   `json:"location_accuracy_meters,omitempty"`
	LocationUpdatedAt      *time.Time `json:"location_updated_at,omitempty"`
}

type UpdateTenantLocationRequest struct {
	Address       string   `json:"address" binding:"required,max=500"`
	Latitude      *float64 `json:"latitude,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	GooglePlaceID *string  `json:"google_place_id,omitempty,max=255"`
}

func (r UpdateTenantLocationRequest) Validate() error {
	if (r.Latitude == nil) != (r.Longitude == nil) {
		return fmt.Errorf("latitude and longitude must be provided together")
	}
	if r.Latitude != nil && (*r.Latitude < -90 || *r.Latitude > 90) {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if r.Longitude != nil && (*r.Longitude < -180 || *r.Longitude > 180) {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	return nil
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
	GetTenantLocation(ctx context.Context, id uuid.UUID) (*TenantLocation, error)
	UpdateTenantLocation(ctx context.Context, id uuid.UUID, req *UpdateTenantLocationRequest) (*TenantLocation, error)
}
