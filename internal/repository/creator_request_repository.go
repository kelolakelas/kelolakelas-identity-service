package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type CreatorRequestRepository interface {
	Create(ctx context.Context, tenantID, requesterID, callerRoleID uuid.UUID, email string, targetUserID *uuid.UUID, reason string) (*domain.CreatorRequest, error)
	List(ctx context.Context, tenantID, requesterID, callerRoleID uuid.UUID) ([]domain.CreatorRequest, error)
}

type creatorRequestRepository struct{ db *gorm.DB }

func NewCreatorRequestRepository(db *gorm.DB) CreatorRequestRepository {
	return &creatorRequestRepository{db: db}
}

// Authorization checks the live membership, not the potentially stale JWT role.
func (r *creatorRequestRepository) creator(ctx context.Context, db *gorm.DB, tenantID, requesterID, roleID uuid.UUID) error {
	var memberID string
	// The row lock keeps a concurrent role change/removal from committing between
	// checking the Creator membership and writing/reading the request.
	err := db.WithContext(ctx).Raw(`SELECT tm.id FROM tenant_members tm
		JOIN roles ro ON ro.id = tm.role_id
		WHERE tm.tenant_id = ? AND tm.user_id = ? AND tm.role_id = ?
		AND tm.is_active = true AND tm.deleted_at IS NULL
		AND ro.name = 'Creator' AND ro.tenant_id IS NULL
		FOR SHARE OF tm`, tenantID, requesterID, roleID).Scan(&memberID).Error
	if err != nil {
		return err
	}
	if memberID == "" {
		return domain.ErrCreatorRequestForbidden
	}
	return nil
}

func (r *creatorRequestRepository) Create(ctx context.Context, tenantID, requesterID, callerRoleID uuid.UUID, email string, targetUserID *uuid.UUID, reason string) (*domain.CreatorRequest, error) {
	var request domain.CreatorRequest
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.creator(ctx, tx, tenantID, requesterID, callerRoleID); err != nil {
			return err
		}
		var user domain.User
		if targetUserID != nil {
			if err := tx.Where("id = ? AND deleted_at IS NULL", *targetUserID).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.ErrCreatorRequestInvalid
				}
				return err
			}
			if email != "" && !strings.EqualFold(email, user.Email) {
				return domain.ErrCreatorRequestInvalid
			}
			email = strings.ToLower(strings.TrimSpace(user.Email))
		} else {
			err := tx.Where("LOWER(email) = ? AND deleted_at IS NULL", email).First(&user).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				targetUserID = &user.ID
			}
		}
		if targetUserID != nil {
			var count int64
			if err := tx.Table("tenant_members tm").Joins("JOIN roles ro ON ro.id = tm.role_id").
				Where("tm.tenant_id = ? AND tm.user_id = ? AND tm.is_active = true AND tm.deleted_at IS NULL", tenantID, *targetUserID).
				Where("ro.name = ? AND ro.tenant_id IS NULL", "Creator").Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return domain.ErrCreatorTargetAlreadyCreator
			}
		}
		now := time.Now().UTC()
		request = domain.CreatorRequest{ID: uuid.New(), TenantID: tenantID, RequesterUserID: requesterID, TargetEmail: email, TargetUserID: targetUserID, Reason: reason, Status: "pending", CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&request).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_creator_requests_pending_target" {
				return domain.ErrCreatorRequestDuplicate
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *creatorRequestRepository) List(ctx context.Context, tenantID, requesterID, callerRoleID uuid.UUID) ([]domain.CreatorRequest, error) {
	var requests []domain.CreatorRequest
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.creator(ctx, tx, tenantID, requesterID, callerRoleID); err != nil {
			return err
		}
		return tx.Where("tenant_id = ?", tenantID).Order("created_at DESC, id DESC").Find(&requests).Error
	})
	if err != nil {
		return nil, err
	}
	return requests, nil
}
