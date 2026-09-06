CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email varchar(255) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL, first_name varchar(255) NOT NULL, last_name varchar(255) NOT NULL,
    phone varchar(50), is_parent boolean NOT NULL DEFAULT false,
    created_at timestamp NOT NULL DEFAULT now(), updated_at timestamp NOT NULL DEFAULT now(), deleted_at timestamp
);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

CREATE TABLE IF NOT EXISTS tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name varchar(255) NOT NULL UNIQUE, phone varchar(50), address text,
    address_formatted varchar(500), latitude decimal(10,7), longitude decimal(10,7), google_place_id varchar(255),
    location_accuracy_meters decimal, location_updated_at timestamp, about jsonb, payment_account_id varchar(255) UNIQUE,
    status varchar(50) NOT NULL DEFAULT 'active', created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now(), deleted_at timestamp,
    CONSTRAINT tenants_latitude_range CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CONSTRAINT tenants_longitude_range CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180),
    CONSTRAINT tenants_coordinates_together CHECK ((latitude IS NULL AND longitude IS NULL) OR (latitude IS NOT NULL AND longitude IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_tenants_deleted_at ON tenants (deleted_at);
CREATE INDEX IF NOT EXISTS idx_tenants_latitude_longitude ON tenants (latitude, longitude);

CREATE TABLE IF NOT EXISTS roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid, name varchar(255) NOT NULL, description text,
    created_at timestamp NOT NULL DEFAULT now(), updated_at timestamp NOT NULL DEFAULT now(),
    CONSTRAINT uq_roles_tenant_name UNIQUE (tenant_id, name), CONSTRAINT fk_roles_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE TABLE IF NOT EXISTS permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name varchar(255) NOT NULL UNIQUE, description text,
    created_at timestamp NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tenant_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, user_id uuid NOT NULL, role_id uuid NOT NULL,
    is_active boolean NOT NULL DEFAULT true, joined_at timestamp NOT NULL DEFAULT now(), updated_at timestamp NOT NULL DEFAULT now(), deleted_at timestamp,
    CONSTRAINT uq_tenant_members_tenant_user UNIQUE (tenant_id, user_id),
    CONSTRAINT fk_tenant_members_tenant FOREIGN KEY (tenant_id) REFERENCES tenants(id),
    CONSTRAINT fk_tenant_members_user FOREIGN KEY (user_id) REFERENCES users(id), CONSTRAINT fk_tenant_members_role FOREIGN KEY (role_id) REFERENCES roles(id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_members_deleted_at ON tenant_members (deleted_at);
CREATE TABLE IF NOT EXISTS role_permissions (
    role_id uuid NOT NULL, permission_id uuid NOT NULL, assigned_at timestamp NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id), FOREIGN KEY (role_id) REFERENCES roles(id), FOREIGN KEY (permission_id) REFERENCES permissions(id)
);
CREATE TABLE IF NOT EXISTS tenant_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, role_id uuid NOT NULL,
    email varchar(255) NOT NULL, token varchar(255) NOT NULL UNIQUE, is_used boolean NOT NULL DEFAULT false,
    expires_at timestamp NOT NULL, created_at timestamp NOT NULL DEFAULT now(), updated_at timestamp NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id), FOREIGN KEY (role_id) REFERENCES roles(id)
);
CREATE TABLE IF NOT EXISTS tenant_wallets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL UNIQUE, available_balance bigint NOT NULL DEFAULT 0,
    pending_balance bigint NOT NULL DEFAULT 0, updated_at timestamp NOT NULL DEFAULT now(), FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE TABLE IF NOT EXISTS user_wallets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL UNIQUE, balance bigint NOT NULL DEFAULT 0,
    updated_at timestamp NOT NULL DEFAULT now(), FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE TABLE IF NOT EXISTS tenant_bank_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, bank_code varchar(50) NOT NULL,
    account_number varchar(50) NOT NULL, account_name varchar(255) NOT NULL, is_primary boolean NOT NULL DEFAULT true,
    created_at timestamp NOT NULL DEFAULT now(), deleted_at timestamp, FOREIGN KEY (tenant_id) REFERENCES tenants(id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_bank_accounts_deleted_at ON tenant_bank_accounts (deleted_at);
CREATE TABLE IF NOT EXISTS user_bank_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL, bank_code varchar(50) NOT NULL,
    account_number varchar(50) NOT NULL, account_name varchar(255) NOT NULL, is_primary boolean NOT NULL DEFAULT true,
    created_at timestamp NOT NULL DEFAULT now(), deleted_at timestamp, FOREIGN KEY (user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_user_bank_accounts_deleted_at ON user_bank_accounts (deleted_at);
CREATE TABLE IF NOT EXISTS tenant_ledger_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_wallet_id uuid NOT NULL, reference_id uuid,
    reference_type varchar(50) NOT NULL, amount bigint NOT NULL, entry_type varchar(50) NOT NULL, description text,
    created_at timestamp NOT NULL DEFAULT now(), FOREIGN KEY (tenant_wallet_id) REFERENCES tenant_wallets(id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_ledger_entries_wallet ON tenant_ledger_entries (tenant_wallet_id);
CREATE INDEX IF NOT EXISTS idx_tenant_ledger_entries_reference ON tenant_ledger_entries (reference_type, reference_id);
CREATE TABLE IF NOT EXISTS user_ledger_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_wallet_id uuid NOT NULL, reference_id uuid,
    reference_type varchar(50) NOT NULL, amount bigint NOT NULL, entry_type varchar(50) NOT NULL, description text,
    created_at timestamp NOT NULL DEFAULT now(), FOREIGN KEY (user_wallet_id) REFERENCES user_wallets(id)
);
CREATE INDEX IF NOT EXISTS idx_user_ledger_entries_wallet ON user_ledger_entries (user_wallet_id);
CREATE INDEX IF NOT EXISTS idx_user_ledger_entries_reference ON user_ledger_entries (reference_type, reference_id);
CREATE TABLE IF NOT EXISTS tenant_withdrawals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL, tenant_bank_account_id uuid NOT NULL,
    amount bigint NOT NULL, admin_fee bigint NOT NULL DEFAULT 0, net_amount bigint NOT NULL, status varchar(50) NOT NULL,
    provider_payout_id varchar(255) UNIQUE, requested_at timestamp NOT NULL DEFAULT now(), processed_at timestamp,
    FOREIGN KEY (tenant_id) REFERENCES tenants(id), FOREIGN KEY (tenant_bank_account_id) REFERENCES tenant_bank_accounts(id)
);
CREATE INDEX IF NOT EXISTS idx_tenant_withdrawals_tenant_status ON tenant_withdrawals (tenant_id, status);
CREATE TABLE IF NOT EXISTS user_withdrawals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL, user_bank_account_id uuid NOT NULL,
    amount bigint NOT NULL, admin_fee bigint NOT NULL DEFAULT 0, net_amount bigint NOT NULL, status varchar(50) NOT NULL,
    provider_payout_id varchar(255) UNIQUE, requested_at timestamp NOT NULL DEFAULT now(), processed_at timestamp,
    FOREIGN KEY (user_id) REFERENCES users(id), FOREIGN KEY (user_bank_account_id) REFERENCES user_bank_accounts(id)
);
CREATE INDEX IF NOT EXISTS idx_user_withdrawals_user_status ON user_withdrawals (user_id, status);

CREATE TABLE IF NOT EXISTS seed_versions (filename varchar(255) PRIMARY KEY, checksum varchar(64) NOT NULL, applied_at timestamp NOT NULL DEFAULT now());
