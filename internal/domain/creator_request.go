package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrCreatorRequestForbidden     = errors.New("active creator membership required")
	ErrCreatorRequestDuplicate     = errors.New("pending creator request already exists")
	ErrCreatorTargetAlreadyCreator = errors.New("target is already a creator")
	ErrCreatorRequestInvalid       = errors.New("invalid creator request target or reason")
	ErrCreatorRequestNotFound      = errors.New("creator request not found")
	ErrCreatorRequestDecided       = errors.New("creator request already decided")
	ErrCreatorRequestStale         = errors.New("creator request target or tenant no longer valid")
)

type CreatorRequest struct {
	ID              uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey"`
	TenantID        uuid.UUID  `json:"tenant_id" gorm:"type:uuid;not null"`
	RequesterUserID uuid.UUID  `json:"requester_user_id" gorm:"type:uuid;not null"`
	TargetEmail     string     `json:"target_email" gorm:"not null"`
	TargetUserID    *uuid.UUID `json:"target_user_id,omitempty" gorm:"type:uuid"`
	Reason          string     `json:"reason" gorm:"not null"`
	Status          string     `json:"status" gorm:"not null"`
	DecidedBy       *uuid.UUID `json:"decided_by,omitempty" gorm:"type:uuid"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	InvitationID    *uuid.UUID `json:"invitation_id,omitempty" gorm:"type:uuid"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (CreatorRequest) TableName() string { return "creator_requests" }
