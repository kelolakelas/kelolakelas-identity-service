package repository

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// This DSN must point to a disposable database; apply the real migration chain.
func TestKEL180EnrollmentVersionAndGuards(t *testing.T) {
	dsn := os.Getenv("KEL180_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL180_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	dir, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New((&url.URL{Scheme: "file", Path: dir}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	id := uuid.New()
	if err := db.Exec("INSERT INTO users(id,email,password_hash,first_name,last_name) VALUES(?,?,'hash','Admin','Test')", id, id.String()+"@example.test").Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id = ?", id)
	if err := db.Exec("INSERT INTO platform_admin_assignments(user_id,enrollment_allowed,factor_version) VALUES (?,true,1)", id).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM platform_admin_assignments WHERE user_id = ?", id)
	defer db.Exec("DELETE FROM platform_factor_challenges WHERE user_id = ?", id)
	ctx := context.Background()
	store := NewPlatformFactorStore(db, make([]byte, 32))
	now := time.Now().UTC()
	verify := func(secret []byte) (bool, error) { return string(secret) == "secret", nil }
	begin := func(purpose string, token []byte, expires time.Time) {
		t.Helper()
		if _, err := store.Begin(ctx, id, purpose, token, []byte("secret"), expires); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Version(ctx, id); err == nil {
		t.Fatal("password-only assignment has verified version")
	}
	enroll := []byte("first-enroll")
	begin("enroll", enroll, now.Add(time.Minute))
	ok, version, err := store.Consume(ctx, id, "enroll", enroll, now, verify)
	if err != nil || !ok {
		t.Fatalf("enroll: %v %v", ok, err)
	}
	persisted, err := store.Version(ctx, id)
	if err != nil || version != persisted || version != 2 {
		t.Fatalf("returned=%d persisted=%d err=%v", version, persisted, err)
	}
	if ok, _, err := store.Consume(ctx, id, "enroll", enroll, now, verify); err != nil || ok {
		t.Fatalf("replay: %v %v", ok, err)
	}
	login := []byte("next-login")
	begin("verify", login, now.Add(time.Minute))
	if ok, v, err := store.Consume(ctx, id, "verify", login, now, verify); err != nil || !ok || v != persisted {
		t.Fatalf("login: %v %d %v", ok, v, err)
	}
	expired := []byte("expired")
	begin("verify", expired, now.Add(-time.Minute))
	if ok, _, err := store.Consume(ctx, id, "verify", expired, now, verify); err != nil || ok {
		t.Fatalf("expired: %v %v", ok, err)
	}
	capped := []byte("attempt-cap")
	begin("verify", capped, now.Add(time.Minute))
	for i := 0; i < 5; i++ {
		if ok, _, err := store.Consume(ctx, id, "verify", capped, now, func([]byte) (bool, error) { return false, nil }); err != nil || ok {
			t.Fatalf("invalid attempt: %v %v", ok, err)
		}
	}
	if ok, _, err := store.Consume(ctx, id, "verify", capped, now, verify); err != nil || ok {
		t.Fatalf("cap: %v %v", ok, err)
	}
	revoked := []byte("revoked")
	begin("verify", revoked, now.Add(time.Minute))
	if err := db.Exec("UPDATE platform_admin_assignments SET is_active=false WHERE user_id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if ok, _, err := store.Consume(ctx, id, "verify", revoked, now, verify); err == nil || ok {
		t.Fatalf("revoked consume: %v %v", ok, err)
	}
	if _, err := store.Version(ctx, id); err == nil {
		t.Fatal("revoked version accepted")
	}
}
