package domain

import (
	"time"

	"github.com/google/uuid"
)

// PlatformCreatorRequest exposes only fields needed for a cross-tenant decision.
type PlatformCreatorRequest struct {
	ID              uuid.UUID  `json:"id"`
	TenantID        uuid.UUID  `json:"tenant_id"`
	TenantName      string     `json:"tenant_name"`
	RequesterUserID uuid.UUID  `json:"requester_user_id"`
	TargetEmail     string     `json:"target_email"`
	TargetUserID    *uuid.UUID `json:"target_user_id,omitempty"`
	Reason          string     `json:"reason"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DecidedBy       *uuid.UUID `json:"decided_by,omitempty"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
}
