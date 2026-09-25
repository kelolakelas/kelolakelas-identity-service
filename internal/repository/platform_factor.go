package repository

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errFactorUnavailable = errors.New("platform factor unavailable")

type platformFactorStore struct {
	db  *gorm.DB
	key []byte
}

func NewPlatformFactorStore(db *gorm.DB, key []byte) domain.PlatformFactorStore {
	return &platformFactorStore{db: db, key: key}
}
func (s *platformFactorStore) seal(value []byte) ([]byte, error) {
	if len(s.key) != 32 {
		return nil, errFactorUnavailable
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, value, nil), nil
}
func (s *platformFactorStore) open(value []byte) ([]byte, error) {
	if len(s.key) != 32 {
		return nil, errFactorUnavailable
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(value) < aead.NonceSize() {
		return nil, errFactorUnavailable
	}
	return aead.Open(nil, value[:aead.NonceSize()], value[aead.NonceSize():], nil)
}

type factorAssignment struct {
	UserID            uuid.UUID `gorm:"primaryKey"`
	IsActive          bool
	FactorSecret      []byte
	EnrollmentAllowed bool
	FactorVersion     int64
}

func (factorAssignment) TableName() string { return "platform_admin_assignments" }

type factorChallengeRow struct {
	TokenHash     []byte `gorm:"primaryKey"`
	UserID        uuid.UUID
	Purpose       string
	PendingSecret []byte
	FactorVersion int64
	ExpiresAt     time.Time
	Attempts      int
	ConsumedAt    *time.Time
}

func (factorChallengeRow) TableName() string { return "platform_factor_challenges" }
func (s *platformFactorStore) Begin(ctx context.Context, id uuid.UUID, purpose string, token, secret []byte, expires time.Time) (int64, error) {
	if len(s.key) != 32 {
		return 0, errFactorUnavailable
	}
	var version int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a factorAssignment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", id).First(&a).Error; err != nil {
			return err
		}
		if !a.IsActive {
			return errFactorUnavailable
		}
		if purpose == "enroll" {
			if !a.EnrollmentAllowed || len(a.FactorSecret) != 0 {
				return errFactorUnavailable
			}
		} else if purpose == "verify" {
			if len(a.FactorSecret) == 0 {
				return errFactorUnavailable
			}
		} else {
			return errFactorUnavailable
		}
		var encrypted []byte
		var err error
		if purpose == "enroll" {
			encrypted, err = s.seal(secret)
			if err != nil {
				return err
			}
		}
		h := sha256.Sum256(token)
		version = a.FactorVersion
		return tx.Create(&factorChallengeRow{TokenHash: h[:], UserID: id, Purpose: purpose, PendingSecret: encrypted, FactorVersion: version, ExpiresAt: expires}).Error
	})
	return version, err
}
func (s *platformFactorStore) Consume(ctx context.Context, id uuid.UUID, purpose string, token []byte, now time.Time, verify func([]byte) (bool, error)) (bool, int64, error) {
	if len(s.key) != 32 {
		return false, 0, errFactorUnavailable
	}
	var accepted bool
	var version int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var a factorAssignment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", id).First(&a).Error; err != nil {
			return err
		}
		if !a.IsActive {
			return errFactorUnavailable
		}
		h := sha256.Sum256(token)
		var ch factorChallengeRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ? AND user_id = ? AND purpose = ?", h[:], id, purpose).First(&ch).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if ch.ConsumedAt != nil || !ch.ExpiresAt.After(now) || ch.Attempts >= 5 || ch.FactorVersion != a.FactorVersion {
			return nil
		}
		var secret []byte
		if purpose == "enroll" {
			if !a.EnrollmentAllowed || len(a.FactorSecret) != 0 {
				return nil
			}
			secret, err = s.open(ch.PendingSecret)
		} else if purpose == "verify" {
			if len(a.FactorSecret) == 0 {
				return nil
			}
			secret, err = s.open(a.FactorSecret)
		} else {
			return nil
		}
		if err != nil {
			return err
		}
		ok, err := verify(secret)
		if err != nil {
			return err
		}
		if !ok {
			return tx.Model(&ch).Update("attempts", ch.Attempts+1).Error
		}
		if purpose == "enroll" {
			if err := tx.Model(&a).Updates(map[string]any{"factor_secret": ch.PendingSecret, "enrollment_allowed": false, "factor_version": a.FactorVersion + 1}).Error; err != nil {
				return err
			}
			version = a.FactorVersion + 1
		} else {
			version = a.FactorVersion
		}
		if err := tx.Model(&ch).Updates(map[string]any{"consumed_at": now, "pending_secret": nil}).Error; err != nil {
			return err
		}
		accepted = true
		return nil
	})
	return accepted, version, err
}
func (s *platformFactorStore) AssignmentVersion(ctx context.Context, id uuid.UUID) (int64, error) {
	if len(s.key) != 32 {
		return 0, errFactorUnavailable
	}
	var a factorAssignment
	err := s.db.WithContext(ctx).Where("user_id = ?", id).First(&a).Error
	if err != nil {
		return 0, err
	}
	if !a.IsActive {
		return 0, errFactorUnavailable
	}
	return a.FactorVersion, nil
}
func (s *platformFactorStore) Version(ctx context.Context, id uuid.UUID) (int64, error) {
	if len(s.key) != 32 {
		return 0, errFactorUnavailable
	}
	var a factorAssignment
	err := s.db.WithContext(ctx).Where("user_id = ?", id).First(&a).Error
	if err != nil {
		return 0, err
	}
	if !a.IsActive || len(a.FactorSecret) == 0 {
		return 0, errFactorUnavailable
	}
	return a.FactorVersion, nil
}
