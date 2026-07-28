package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type TenantWallet struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID         uuid.UUID `gorm:"type:uuid;unique;not null" json:"tenant_id"`
	AvailableBalance int64     `gorm:"type:bigint;default:0" json:"available_balance"`
	PendingBalance   int64     `gorm:"type:bigint;default:0" json:"pending_balance"`
	UpdatedAt        time.Time `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
}

func (TenantWallet) TableName() string {
	return "tenant_wallets"
}

type UserWallet struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    uuid.UUID `gorm:"type:uuid;unique;not null" json:"user_id"`
	Balance   int64     `gorm:"type:bigint;default:0" json:"balance"`
	UpdatedAt time.Time `gorm:"type:timestamp;not null;default:now()" json:"updated_at"`
}

func (UserWallet) TableName() string {
	return "user_wallets"
}

type TenantBankAccount struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID      uuid.UUID      `gorm:"type:uuid;not null" json:"tenant_id"`
	BankCode      string         `gorm:"type:varchar(50);not null" json:"bank_code"`
	AccountNumber string         `gorm:"type:varchar(50);not null" json:"account_number"`
	AccountName   string         `gorm:"type:varchar(255);not null" json:"account_name"`
	IsPrimary     bool           `gorm:"type:boolean;default:true" json:"is_primary"`
	CreatedAt     time.Time      `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (TenantBankAccount) TableName() string {
	return "tenant_bank_accounts"
}

type UserBankAccount struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uuid.UUID      `gorm:"type:uuid;not null" json:"user_id"`
	BankCode      string         `gorm:"type:varchar(50);not null" json:"bank_code"`
	AccountNumber string         `gorm:"type:varchar(50);not null" json:"account_number"`
	AccountName   string         `gorm:"type:varchar(255);not null" json:"account_name"`
	IsPrimary     bool           `gorm:"type:boolean;default:true" json:"is_primary"`
	CreatedAt     time.Time      `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (UserBankAccount) TableName() string {
	return "user_bank_accounts"
}

type TenantLedgerEntry struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantWalletID uuid.UUID  `gorm:"type:uuid;not null;index" json:"tenant_wallet_id"`
	ReferenceID    *uuid.UUID `gorm:"type:uuid" json:"reference_id,omitempty"`
	ReferenceType  string     `gorm:"type:varchar(50);not null" json:"reference_type"`
	Amount         int64      `gorm:"type:bigint;not null" json:"amount"`
	EntryType      string     `gorm:"type:varchar(50);not null" json:"entry_type"`
	Description    *string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt      time.Time  `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
}

func (TenantLedgerEntry) TableName() string {
	return "tenant_ledger_entries"
}

type UserLedgerEntry struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserWalletID  uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_wallet_id"`
	ReferenceID   *uuid.UUID `gorm:"type:uuid" json:"reference_id,omitempty"`
	ReferenceType string     `gorm:"type:varchar(50);not null" json:"reference_type"`
	Amount        int64      `gorm:"type:bigint;not null" json:"amount"`
	EntryType     string     `gorm:"type:varchar(50);not null" json:"entry_type"`
	Description   *string    `gorm:"type:text" json:"description,omitempty"`
	CreatedAt     time.Time  `gorm:"type:timestamp;not null;default:now()" json:"created_at"`
}

func (UserLedgerEntry) TableName() string {
	return "user_ledger_entries"
}

type TenantWithdrawal struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID            uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_id"`
	TenantBankAccountID uuid.UUID  `gorm:"type:uuid;not null" json:"tenant_bank_account_id"`
	Amount              int64      `gorm:"type:bigint;not null" json:"amount"`
	AdminFee            int64      `gorm:"type:bigint;default:0" json:"admin_fee"`
	NetAmount           int64      `gorm:"type:bigint;not null" json:"net_amount"`
	Status              string     `gorm:"type:varchar(50);not null" json:"status"`
	ProviderPayoutID    *string    `gorm:"type:varchar(255);unique" json:"provider_payout_id,omitempty"`
	RequestedAt         time.Time  `gorm:"type:timestamp;not null;default:now()" json:"requested_at"`
	ProcessedAt         *time.Time `gorm:"type:timestamp" json:"processed_at,omitempty"`
}

func (TenantWithdrawal) TableName() string {
	return "tenant_withdrawals"
}

type UserWithdrawal struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID            uuid.UUID  `gorm:"type:uuid;not null" json:"user_id"`
	UserBankAccountID uuid.UUID  `gorm:"type:uuid;not null" json:"user_bank_account_id"`
	Amount            int64      `gorm:"type:bigint;not null" json:"amount"`
	AdminFee          int64      `gorm:"type:bigint;default:0" json:"admin_fee"`
	NetAmount         int64      `gorm:"type:bigint;not null" json:"net_amount"`
	Status            string     `gorm:"type:varchar(50);not null" json:"status"`
	ProviderPayoutID  *string    `gorm:"type:varchar(255);unique" json:"provider_payout_id,omitempty"`
	RequestedAt       time.Time  `gorm:"type:timestamp;not null;default:now()" json:"requested_at"`
	ProcessedAt       *time.Time `gorm:"type:timestamp" json:"processed_at,omitempty"`
}

func (UserWithdrawal) TableName() string {
	return "user_withdrawals"
}
