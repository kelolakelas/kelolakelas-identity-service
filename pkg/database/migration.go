package database

import (
	"fmt"
	"strings"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	if err := db.AutoMigrate(
		&domain.User{},
		&domain.Tenant{},
		&domain.TenantMember{},
		&domain.Role{},
		&domain.Permission{},
		&domain.RolePermission{},
		&domain.TenantInvitation{},
		&domain.TenantWallet{},
		&domain.UserWallet{},
		&domain.TenantBankAccount{},
		&domain.UserBankAccount{},
		&domain.TenantLedgerEntry{},
		&domain.UserLedgerEntry{},
		&domain.TenantWithdrawal{},
		&domain.UserWithdrawal{},
	); err != nil {
		return err
	}
	for _, statement := range []string{
		`ALTER TABLE tenants ADD CONSTRAINT tenants_latitude_range CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90)`,
		`ALTER TABLE tenants ADD CONSTRAINT tenants_longitude_range CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)`,
		`ALTER TABLE tenants ADD CONSTRAINT tenants_coordinates_together CHECK ((latitude IS NULL AND longitude IS NULL) OR (latitude IS NOT NULL AND longitude IS NOT NULL))`,
		`CREATE INDEX IF NOT EXISTS idx_tenants_latitude_longitude ON tenants (latitude, longitude)`,
	} {
		if err := db.Exec(statement).Error; err != nil && !isAlreadyExistsError(err) {
			return err
		}
	}
	return nil
}

func isAlreadyExistsError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "already exists") || strings.Contains(message, "duplicate")
}
