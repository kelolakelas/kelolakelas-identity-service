package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

var ErrPlatformForbidden = errors.New("platform assignment is inactive")
var ErrPlatformUnavailable = errors.New("platform factor unavailable")

type PlatformAuth struct {
	users  domain.UserRepository
	admins domain.PlatformAdminRepository
	tokens *jwt.JWTService
	factor domain.PlatformFactorStore
}

func NewPlatformAuth(users domain.UserRepository, admins domain.PlatformAdminRepository, tokens *jwt.JWTService) *PlatformAuth {
	return &PlatformAuth{users: users, admins: admins, tokens: tokens}
}

func (a *PlatformAuth) Login(ctx context.Context, email, password string) (string, error) {
	user, err := a.users.GetByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		return "", domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	if !hash.CheckPasswordHash(password, user.PasswordHash) {
		return "", domain.ErrInvalidCredentials
	}
	if err := a.Check(ctx, user.ID); err != nil {
		return "", err
	}
	var validAfter *time.Time
	if store, ok := a.users.(interface {
		SessionValidAfter(context.Context, uuid.UUID) (*time.Time, error)
	}); ok {
		validAfter, err = store.SessionValidAfter(ctx, user.ID)
		if err != nil {
			return "", err
		}
	}
	if a.factor == nil {
		return "", ErrPlatformUnavailable
	}
	// The password stage is never a platform principal. Recovery can only be
	// authorized by the operator; enrollment itself requires a DB gate.
	version, err := a.factor.AssignmentVersion(ctx, user.ID)
	if err != nil {
		return "", ErrPlatformUnavailable
	}
	_ = validAfter
	return a.tokens.GeneratePendingPlatformToken(user.ID, user.Email, version)
}

func (a *PlatformAuth) CheckVersion(ctx context.Context, userID uuid.UUID, version int64) error {
	if version <= 0 || a.factor == nil {
		return ErrPlatformForbidden
	}
	if err := a.Check(ctx, userID); err != nil {
		return err
	}
	current, err := a.factor.Version(ctx, userID)
	if err != nil {
		return ErrPlatformUnavailable
	}
	if current != version {
		return ErrPlatformForbidden
	}
	return nil
}

func (a *PlatformAuth) Check(ctx context.Context, userID uuid.UUID) error {
	if userID == uuid.Nil {
		return ErrPlatformForbidden
	}
	active, err := a.admins.IsActive(ctx, userID)
	if err != nil {
		return err
	}
	if !active {
		return ErrPlatformForbidden
	}
	_, err = a.users.GetByID(ctx, userID)
	if errors.Is(err, domain.ErrUserNotFound) {
		return ErrPlatformForbidden
	}
	return err
}
