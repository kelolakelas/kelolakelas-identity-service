package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type FactorChallenge struct {
	UserID    uuid.UUID
	Purpose   string
	Secret    []byte
	Version   int64
	ExpiresAt time.Time
	Attempts  int
}

// PlatformFactorStore serializes challenge consumption with assignment and factor updates.
type PlatformFactorStore interface {
	Begin(context.Context, uuid.UUID, string, []byte, []byte, time.Time) (int64, error)
	Consume(context.Context, uuid.UUID, string, []byte, time.Time, func([]byte) (bool, error)) (bool, int64, error)
	Version(context.Context, uuid.UUID) (int64, error)
	AssignmentVersion(context.Context, uuid.UUID) (int64, error)
}
