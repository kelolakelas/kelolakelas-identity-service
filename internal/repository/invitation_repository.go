package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm/clause"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type invitationRepository struct {
	db *gorm.DB
}

func NewInvitationRepository(db *gorm.DB) InvitationRepository {
	return &invitationRepository{db: db}
}

func (r *invitationRepository) Create(ctx context.Context, invitation *domain.TenantInvitation) error {
	return r.db.WithContext(ctx).Create(invitation).Error
}

func (r *invitationRepository) GetByToken(ctx context.Context, token string) (*domain.TenantInvitation, error) {
	var invitation domain.TenantInvitation
	if err := r.db.WithContext(ctx).Where("token = ?", token).First(&invitation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrInvitationNotFound
		}
		return nil, err
	}
	return &invitation, nil
}

func (r *invitationRepository) GetByTenantAndEmail(ctx context.Context, tenantID uuid.UUID, email string) (*domain.TenantInvitation, error) {
	var invitation domain.TenantInvitation
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND email = ? AND is_used = ?", tenantID, email, false).First(&invitation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrInvitationNotFound
		}
		return nil, err
	}
	return &invitation, nil
}

func (r *invitationRepository) Update(ctx context.Context, invitation *domain.TenantInvitation) error {
	return r.db.WithContext(ctx).Save(invitation).Error
}

// ReplaceActive serializes invitations for a tenant via its parent row. This also
// closes the gap where two concurrent requests both see no active invitation.
func (r *invitationRepository) ReplaceActive(ctx context.Context, invitation *domain.TenantInvitation) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant domain.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tenant, "id = ?", invitation.TenantID).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.TenantInvitation{}).
			Where("tenant_id = ? AND LOWER(email) = LOWER(?) AND is_used = ?", invitation.TenantID, invitation.Email, false).
			Updates(map[string]interface{}{"is_used": true, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return tx.Create(invitation).Error
	})
}

func (r *invitationRepository) ListPending(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantInvitation, error) {
	var invitations []domain.TenantInvitation
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND is_used = ?", tenantID, false).
		Order("created_at DESC").Find(&invitations).Error
	return invitations, err
}

func (r *invitationRepository) Revoke(ctx context.Context, tenantID, invitationID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&domain.TenantInvitation{}).
		Where("id = ? AND tenant_id = ? AND is_used = ?", invitationID, tenantID, false).
		Updates(map[string]interface{}{"is_used": true, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrInvitationNotFound
	}
	return nil
}
