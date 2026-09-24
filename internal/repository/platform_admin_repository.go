package repository

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type platformAdminRepository struct{ db *gorm.DB }

func NewPlatformAdminRepository(db *gorm.DB) *platformAdminRepository {
	return &platformAdminRepository{db: db}
}

func (r *platformAdminRepository) IsActive(ctx context.Context, userID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("platform_admin_assignments").
		Where("user_id = ? AND is_active = true", userID).Count(&count).Error
	return count > 0, err
}
