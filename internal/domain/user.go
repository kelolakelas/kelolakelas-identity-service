package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

type HTTPResponse struct {
	Status  string      `json:"status" example:"success"`
	Message string      `json:"message" example:"Operation completed successfully"`
	Data    interface{} `json:"data,omitempty"`
}

type AuthUserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	IsParent  bool      `json:"is_parent"`
	TenantID  uuid.UUID `json:"tenant_id"`
}

type ErrorResponse struct {
	Status  string      `json:"status" example:"error"`
	Message string      `json:"message" example:"An error occurred"`
	Data    interface{} `json:"data,omitempty"`
}

type User struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email        string         `gorm:"type:varchar(255);unique;not null" json:"email"`
	PasswordHash string         `gorm:"type:varchar(255);not null" json:"-"`
	FirstName    string         `gorm:"type:varchar(255);not null" json:"first_name"`
	LastName     string         `gorm:"type:varchar(255);not null" json:"last_name"`
	Phone        *string        `gorm:"type:varchar(50)" json:"phone,omitempty"`
	IsParent     bool           `gorm:"type:boolean;default:false" json:"is_parent"`
	CreatedAt    time.Time      `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uuid.UUID) error
	RegisterTenantTx(ctx context.Context, user *User, tenant *Tenant) (*TenantMember, error)
	RegisterInvitedUserTx(ctx context.Context, token, firstName, lastName, password string) (*User, error)
	GetPermissionsByRoleId(ctx context.Context, roleID uuid.UUID) ([]string, error)
	GetTenantMemberByUserID(ctx context.Context, userID uuid.UUID) (*TenantMember, error)
}

type AuthUsecase interface {
	Register(ctx context.Context, user *User, password string) (*User, error)
	RegisterInvitedUser(ctx context.Context, token, firstName, lastName, password string) (*User, error)
	Login(ctx context.Context, email, password string) (string, *User, uuid.UUID, error)
}
