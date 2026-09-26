package repository

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
)

// KEL23_TEST_DATABASE_URL must point at an isolated, migrated PostgreSQL database.
func TestLoginAttemptsPostgresConcurrentAndRestart(t *testing.T) {
	dsn := os.Getenv("KEL23_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL23_TEST_DATABASE_URL for PostgreSQL integration test")
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

	ctx := context.Background()
	id := uuid.New()
	passwordHash, err := hash.HashPassword("correct")
	if err != nil {
		t.Fatal(err)
	}
	email := "login-" + id.String() + "@example.com"
	if err := db.Exec("INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, ?, ?, ?)", id, email, passwordHash, "Login", "Test").Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id = ?", id)

	store := NewLoginAttemptStore(db)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Authenticate(ctx, email, "wrong", passwordHash, 3, time.Minute)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("failed attempt: %v", err)
		}
	}
	var user domain.User
	if err := db.First(&user, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if user.LoginLockedUntil == nil || user.FailedLoginAttempts != 0 {
		t.Fatalf("concurrent failures did not lock: %+v", user)
	}
	// A new store instance (simulating a restart) sees the committed lockout.
	if _, err := NewLoginAttemptStore(db).Authenticate(ctx, email, "correct", passwordHash, 3, time.Minute); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("restart bypassed lockout: %v", err)
	}
	if err := db.Model(&domain.User{}).Where("id = ?", id).Update("login_locked_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewLoginAttemptStore(db).Authenticate(ctx, email, "wrong", passwordHash, 3, time.Minute); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expired lock attempt: %v", err)
	}
	user = domain.User{}
	if err := db.First(&user, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if user.FailedLoginAttempts != 1 || user.LoginLockedUntil != nil {
		t.Fatalf("expiry did not reset count: %+v", user)
	}
	if _, err := store.Authenticate(ctx, email, "correct", passwordHash, 3, time.Minute); err != nil {
		t.Fatalf("correct password: %v", err)
	}
	user = domain.User{}
	if err := db.First(&user, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if user.FailedLoginAttempts != 0 || user.LoginLockedUntil != nil {
		t.Fatalf("success did not reset: %+v", user)
	}
}
