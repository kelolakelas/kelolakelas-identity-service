package repository

import (
	"context"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

type PlatformCreatorRequestRepository interface {
	ListPlatform(ctx context.Context) ([]domain.PlatformCreatorRequest, error)
}

type platformCreatorRequestRepository struct{ db *gorm.DB }

func NewPlatformCreatorRequestRepository(db *gorm.DB) PlatformCreatorRequestRepository {
	return &platformCreatorRequestRepository{db: db}
}

func (r *platformCreatorRequestRepository) ListPlatform(ctx context.Context) ([]domain.PlatformCreatorRequest, error) {
	requests := make([]domain.PlatformCreatorRequest, 0)
	err := r.db.WithContext(ctx).Table("creator_requests AS cr").
		Select("cr.id, cr.tenant_id, t.name AS tenant_name, cr.requester_user_id, cr.target_email, cr.target_user_id, cr.reason, cr.status, cr.created_at, cr.updated_at, cr.decided_by, cr.decided_at, cr.rejection_reason").
		Joins("JOIN tenants AS t ON t.id = cr.tenant_id").
		Where("cr.status = ?", "pending").
		Order("cr.created_at ASC, cr.id ASC").Scan(&requests).Error
	return requests, err
}
