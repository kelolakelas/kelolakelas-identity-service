package repository

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Set KEL97_TEST_DATABASE_URL to an isolated identity database with migrations.
func kel97DB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("KEL97_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL97_TEST_DATABASE_URL for PostgreSQL integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
func kel97Count(t *testing.T, db *gorm.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}
func kel97Operator(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO users(id,email,password_hash,first_name,last_name) VALUES(?,?,'hash','Admin','Test')`, id, "kel97-admin-"+id.String()+"@example.com").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DELETE FROM users WHERE id = ?", id).Error })
	return id
}
func kel97Version(t *testing.T, db *gorm.DB, operator uuid.UUID, version int64, value string) int64 {
	t.Helper()
	var id int64
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE configuration_heads SET latest_version = ? WHERE application = 'identity' AND environment = 'platform' AND config_key = 'TENANT_REGISTRATION_OPEN'`, version).Error; err != nil {
			return err
		}
		return tx.Raw(`INSERT INTO configuration_versions(application,environment,config_key,version,value,created_by) VALUES('identity','platform','TENANT_REGISTRATION_OPEN',?,?::jsonb,?) RETURNING id`, version, value, operator).Row().Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM configuration_reports WHERE version_id = ?", id).Error
		_ = db.Exec("DELETE FROM configuration_versions WHERE id = ?", id).Error
		_ = db.Exec(`UPDATE configuration_heads SET latest_version = 0 WHERE application = 'identity' AND environment = 'platform' AND config_key = 'TENANT_REGISTRATION_OPEN'`).Error
	})
	return id
}
func kel97NewTenant() (*domain.User, *domain.Tenant) {
	id := uuid.New()
	return &domain.User{ID: id, Email: "kel97-" + id.String() + "@example.com", PasswordHash: "hash", FirstName: "Test", LastName: "User"}, &domain.Tenant{ID: uuid.New(), Name: "kel97-" + uuid.New().String(), Status: "active"}
}
func kel97NoRows(t *testing.T, db *gorm.DB, user *domain.User, tenant *domain.Tenant) {
	t.Helper()
	for _, q := range []struct {
		sql string
		id  uuid.UUID
	}{
		{"SELECT count(*) FROM users WHERE id = ?", user.ID},
		{"SELECT count(*) FROM tenants WHERE id = ?", tenant.ID},
		{"SELECT count(*) FROM user_wallets WHERE user_id = ?", user.ID},
		{"SELECT count(*) FROM tenant_wallets WHERE tenant_id = ?", tenant.ID},
		{"SELECT count(*) FROM tenant_members WHERE tenant_id = ?", tenant.ID},
	} {
		if n := kel97Count(t, db, q.sql, q.id); n != 0 {
			t.Fatalf("partial rows in %q: %d", q.sql, n)
		}
	}
}

func TestRegistrationPolicyPostgresDefaultAndAppliedVersion(t *testing.T) {
	db := kel97DB(t)
	operator := kel97Operator(t, db)
	store := NewConfigurationRepository(db).(*configurationRepository)
	state, err := store.RegistrationState(context.Background())
	if err != nil || !state.Open || state.AppliedVersion != 0 || state.DesiredVersion != 0 {
		t.Fatalf("default: %+v %v", state, err)
	}
	id := kel97Version(t, db, operator, 1, "false")
	state, err = store.RegistrationState(context.Background())
	if err != nil || !state.Open || state.DesiredVersion != 1 || state.AppliedVersion != 0 {
		t.Fatalf("desired only: %+v %v", state, err)
	}
	_, err = store.RecordConfigurationReport(context.Background(), domain.ConfigurationReportRequest{VersionID: id, Status: domain.ConfigurationStatusApplied, ReportedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	state, err = store.RegistrationState(context.Background())
	if err != nil || state.Open || state.DesiredVersion != 1 || state.AppliedVersion != 1 {
		t.Fatalf("applied: %+v %v", state, err)
	}
}

func TestRegisterTenantTxDefaultOpenPostgres(t *testing.T) {
	db := kel97DB(t)
	user, tenant := kel97NewTenant()
	member, err := NewUserRepository(db).RegisterTenantTx(context.Background(), user, tenant)
	if err != nil {
		t.Fatalf("default-open registration: %v", err)
	}
	if member == nil || member.TenantID != tenant.ID || member.UserID != user.ID {
		t.Fatalf("membership missing: %+v", member)
	}
	for _, q := range []struct {
		sql string
		id  uuid.UUID
	}{
		{"SELECT count(*) FROM users WHERE id = ?", user.ID},
		{"SELECT count(*) FROM tenants WHERE id = ?", tenant.ID},
		{"SELECT count(*) FROM user_wallets WHERE user_id = ?", user.ID},
		{"SELECT count(*) FROM tenant_wallets WHERE tenant_id = ?", tenant.ID},
		{"SELECT count(*) FROM tenant_members WHERE tenant_id = ?", tenant.ID},
	} {
		if n := kel97Count(t, db, q.sql, q.id); n != 1 {
			t.Fatalf("successful registration %q rows=%d", q.sql, n)
		}
	}
	db.Exec("DELETE FROM tenant_members WHERE tenant_id = ?", tenant.ID)
	db.Exec("DELETE FROM tenant_wallets WHERE tenant_id = ?", tenant.ID)
	db.Exec("DELETE FROM user_wallets WHERE user_id = ?", user.ID)
	db.Exec("DELETE FROM tenants WHERE id = ?", tenant.ID)
	db.Exec("DELETE FROM users WHERE id = ?", user.ID)
}

func TestRegisterTenantTxClosedAndRollbackPostgres(t *testing.T) {
	db := kel97DB(t)
	operator := kel97Operator(t, db)
	id := kel97Version(t, db, operator, 1, "false")
	store := NewConfigurationRepository(db)
	_, err := store.RecordConfigurationReport(context.Background(), domain.ConfigurationReportRequest{VersionID: id, Status: domain.ConfigurationStatusApplied, ReportedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	user, tenant := kel97NewTenant()
	_, err = NewUserRepository(db).RegisterTenantTx(context.Background(), user, tenant)
	if !errors.Is(err, domain.ErrRegistrationClosed) {
		t.Fatalf("closed: %v", err)
	}
	kel97NoRows(t, db, user, tenant)
}

func TestRegisterTenantTxRacesAppliedClosePostgres(t *testing.T) {
	db := kel97DB(t)
	operator := kel97Operator(t, db)
	id := kel97Version(t, db, operator, 1, "false")
	store := NewConfigurationRepository(db)
	// The registration has entered its transaction while the closing report
	// commits; the lock ordering must yield either a complete registration
	// before close or a closed rejection with zero partial rows.
	const runs = 6
	for i := 0; i < runs; i++ {
		user, tenant := kel97NewTenant()
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var regErr, closeErr error
		go func() {
			defer wg.Done()
			<-start
			_, regErr = NewUserRepository(db).RegisterTenantTx(context.Background(), user, tenant)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, closeErr = store.RecordConfigurationReport(context.Background(), domain.ConfigurationReportRequest{VersionID: id, Status: domain.ConfigurationStatusApplied, ReportedBy: operator})
		}()
		close(start)
		wg.Wait()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if regErr != nil && !errors.Is(regErr, domain.ErrRegistrationClosed) {
			t.Fatalf("registration: %v", regErr)
		}
		if regErr != nil {
			kel97NoRows(t, db, user, tenant)
		} else {
			if n := kel97Count(t, db, "SELECT count(*) FROM tenant_members WHERE tenant_id = ?", tenant.ID); n != 1 {
				t.Fatalf("success missing member: %d", n)
			}
			// Remove in FK order to leave the isolated database clean.
			db.Exec("DELETE FROM tenant_members WHERE tenant_id = ?", tenant.ID)
			db.Exec("DELETE FROM tenant_wallets WHERE tenant_id = ?", tenant.ID)
			db.Exec("DELETE FROM user_wallets WHERE user_id = ?", user.ID)
			db.Exec("DELETE FROM tenants WHERE id = ?", tenant.ID)
			db.Exec("DELETE FROM users WHERE id = ?", user.ID)
		}
	}
}
