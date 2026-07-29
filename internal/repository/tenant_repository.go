package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tutorin-id/tutorin-identity-service/internal/domain"
)

type tenantRepository struct {
	db *gorm.DB
}

func NewTenantRepository(db *gorm.DB) TenantRepository {
	return &tenantRepository{db: db}
}

func (r *tenantRepository) Create(ctx context.Context, tenant *domain.Tenant) error {
	if err := r.db.WithContext(ctx).Create(tenant).Error; err != nil {
		return err
	}
	return nil
}

func (r *tenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	var tenant domain.Tenant
	if err := r.db.WithContext(ctx).First(&tenant, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &tenant, nil
}

func (r *tenantRepository) IsNameExists(ctx context.Context, name string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&domain.Tenant{}).Where("LOWER(name) = LOWER(?)", name).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *tenantRepository) Update(ctx context.Context, tenant *domain.Tenant) error {
	if err := r.db.WithContext(ctx).Save(tenant).Error; err != nil {
		return err
	}
	return nil
}

func (r *tenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.db.WithContext(ctx).Delete(&domain.Tenant{}, "id = ?", id).Error; err != nil {
		return err
	}
	return nil
}
