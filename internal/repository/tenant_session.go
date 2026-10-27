package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenantSessionRepository struct{ db *gorm.DB }

func NewTenantSessionRepository(db *gorm.DB) domain.TenantSessionRepository {
	return &tenantSessionRepository{db: db}
}

func ownTenantChoices(db *gorm.DB, userID uuid.UUID) *gorm.DB {
	return db.Table("tenant_members tm").
		Select("tm.tenant_id, t.name AS tenant_name, tm.id AS member_id, tm.role_id, ro.name AS role_name").
		Joins("JOIN tenants t ON t.id = tm.tenant_id").
		Joins("JOIN roles ro ON ro.id = tm.role_id").
		Where("tm.user_id = ? AND tm.is_active = true AND tm.deleted_at IS NULL", userID).
		Where("t.status = 'active' AND t.deleted_at IS NULL").
		Where("ro.tenant_id IS NULL OR ro.tenant_id = tm.tenant_id")
}

func (r *tenantSessionRepository) List(ctx context.Context, userID uuid.UUID) ([]domain.TenantChoice, error) {
	choices := []domain.TenantChoice{}
	err := ownTenantChoices(r.db.WithContext(ctx), userID).Order("t.name, t.id").Scan(&choices).Error
	return choices, err
}

func (r *tenantSessionRepository) withUser(ctx context.Context, userID uuid.UUID, fn func(*gorm.DB, *domain.User) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user domain.User
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&user, "id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrTenantSessionInvalid
			}
			return err
		}
		return fn(tx, &user)
	})
}

func (r *tenantSessionRepository) WithUser(ctx context.Context, userID uuid.UUID, fn func(*domain.User) error) error {
	return r.withUser(ctx, userID, func(_ *gorm.DB, user *domain.User) error { return fn(user) })
}

func (r *tenantSessionRepository) WithMembership(ctx context.Context, userID, tenantID uuid.UUID, fn func(*domain.User, domain.TenantChoice) error) error {
	return r.withUser(ctx, userID, func(tx *gorm.DB, user *domain.User) error {
		var choice domain.TenantChoice
		// Raw FOR SHARE locks all three inner-joined rows, not merely a stale
		// choice previously sent to the browser. A concurrent update that won
		// the lock is re-evaluated by PostgreSQL before this SELECT returns.
		err := ownTenantChoices(tx, userID).Where("tm.tenant_id = ?", tenantID).
			Clauses(clause.Locking{Strength: "SHARE"}).Take(&choice).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ErrTenantSessionForbidden
		}
		if err != nil {
			return err
		}
		return fn(user, choice)
	})
}
