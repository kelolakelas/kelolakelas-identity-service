package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidResetToken = errors.New("invalid or expired reset token")

type PasswordResetRepository interface {
	Issue(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	Consume(ctx context.Context, tokenHash, passwordHash string, now time.Time) error
	SessionValidAfter(ctx context.Context, userID uuid.UUID) (*time.Time, error)
}
