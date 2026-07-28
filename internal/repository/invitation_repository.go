package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tutorin-id/tutorin-identity-service/internal/domain"
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
