package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type passwordResetRepository struct{ db *gorm.DB }

type passwordResetToken struct {
	TokenHash string `gorm:"primaryKey"`
	UserID    uuid.UUID
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

func (passwordResetToken) TableName() string { return "password_reset_tokens" }

func NewPasswordResetRepository(db *gorm.DB) domain.PasswordResetRepository {
	return &passwordResetRepository{db: db}
}

// Locking the user serializes issuance with consumption, including two requests
// racing to rotate the token. An earlier email may arrive after a newer one;
// only the last committed token can be redeemed.
func (r *passwordResetRepository) Issue(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user domain.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&passwordResetToken{}).Error; err != nil {
			return err
		}
		return tx.Create(&passwordResetToken{TokenHash: tokenHash, UserID: userID, ExpiresAt: expiresAt}).Error
	})
}

// Consume locks the token and the owning user in one transaction. A second
// confirmation cannot observe an unused token until the first commits, and a
// failed password update rolls the consumption back.
func (r *passwordResetRepository) Consume(ctx context.Context, tokenHash, passwordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token passwordResetToken
		if err := tx.Where("token_hash = ?", tokenHash).Take(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInvalidResetToken
			}
			return err
		}
		var user domain.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", token.UserID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInvalidResetToken
			}
			return err
		}
		// Re-read under lock after issuance/another confirmation has committed.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).Take(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInvalidResetToken
			}
			return err
		}
		if token.UsedAt != nil || !time.Now().Before(token.ExpiresAt) {
			return domain.ErrInvalidResetToken
		}
		// JWT iat is integral seconds. The next second is the first admissible
		// issuance boundary; post-reset logins use this same boundary as iat.
		boundary := now.UTC().Truncate(time.Second).Add(time.Second)
		// A second reset can happen before a post-reset token's future iat.
		// Advance strictly so that even those tokens are revoked again.
		if user.SessionValidAfter != nil && !boundary.After(*user.SessionValidAfter) {
			boundary = user.SessionValidAfter.Add(time.Second)
		}
		if err := tx.Model(&user).Updates(map[string]interface{}{"password_hash": passwordHash, "session_valid_after": boundary}).Error; err != nil {
			return err
		}
		if err := tx.Model(&token).Update("used_at", now).Error; err != nil {
			return err
		}
		// Revoke all other outstanding tokens as well.
		return tx.Where("user_id = ? AND token_hash <> ?", user.ID, tokenHash).Delete(&passwordResetToken{}).Error
	})
}

func (r *passwordResetRepository) SessionValidAfter(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).Select("id", "session_valid_after").First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}
	return user.SessionValidAfter, nil
}
