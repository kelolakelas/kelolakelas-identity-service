package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
)

type loginAttemptStore struct {
	db *gorm.DB
}

func NewLoginAttemptStore(db *gorm.DB) *loginAttemptStore {
	return &loginAttemptStore{db: db}
}

// Authenticate serializes password checks and state updates for a single account.
// The row lock also prevents concurrent valid passwords from bypassing a lockout.
func (s *loginAttemptStore) Authenticate(ctx context.Context, email, password, dummyHash string, threshold int, duration time.Duration) (*domain.User, error) {
	var authenticated *domain.User
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user domain.User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(email) = ?", strings.ToLower(email)).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = hash.CheckPasswordHash(password, dummyHash)
			return domain.ErrInvalidCredentials
		}
		if err != nil {
			return err
		}

		now := time.Now()
		// Always do a bcrypt comparison, even while locked, to avoid a fast
		// credential-dependent response path.
		matches := hash.CheckPasswordHash(password, user.PasswordHash)
		if user.LoginLockedUntil != nil && now.Before(*user.LoginLockedUntil) {
			return domain.ErrInvalidCredentials
		}
		if !matches {
			attempts := user.FailedLoginAttempts + 1
			var lockedUntil *time.Time
			if attempts >= threshold {
				until := now.Add(duration)
				lockedUntil = &until
				attempts = 0
			}
			if err := tx.Model(&user).Updates(map[string]interface{}{
				"failed_login_attempts": attempts,
				"login_locked_until":    lockedUntil,
			}).Error; err != nil {
				return err
			}
			// Commit the failed attempt rather than rolling back with the auth error.
			return nil
		}
		if user.FailedLoginAttempts != 0 || user.LoginLockedUntil != nil {
			if err := tx.Model(&user).Updates(map[string]interface{}{
				"failed_login_attempts": 0,
				"login_locked_until":    nil,
			}).Error; err != nil {
				return err
			}
		}
		authenticated = &user
		return nil
	})
	if err != nil {
		return nil, err
	}
	if authenticated == nil {
		return nil, domain.ErrInvalidCredentials
	}
	return authenticated, nil
}
