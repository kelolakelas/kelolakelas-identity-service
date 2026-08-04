package database

import (
	"fmt"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}

	return db.AutoMigrate(
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
	)
}
