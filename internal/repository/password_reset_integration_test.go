package repository

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Set KEL66_TEST_DATABASE_URL to an isolated database with the identity
// migrations applied. This test intentionally exercises PostgreSQL row locks.
func TestPasswordResetPostgresAtomicConsumption(t *testing.T) {
	dsn := os.Getenv("KEL66_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL66_TEST_DATABASE_URL for PostgreSQL integration test")
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
	userID := uuid.New()
	if err := db.Exec("INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, ?, ?, ?)", userID, "reset-"+userID.String()+"@example.com", "oldhash", "Reset", "User").Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id = ?", userID)
	store := NewPasswordResetRepository(db)
	first, second := "first-hash", "second-hash"
	if err := store.Issue(ctx, userID, first, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.Issue(ctx, userID, second, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume(ctx, first, "bad", time.Now()); !errors.Is(err, domain.ErrInvalidResetToken) {
		t.Fatalf("rotated token: %v", err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- store.Consume(ctx, second, "newhash", time.Now()) }()
	}
	wg.Wait()
	close(results)
	good, invalid := 0, 0
	for err := range results {
		switch {
		case err == nil:
			good++
		case errors.Is(err, domain.ErrInvalidResetToken):
			invalid++
		default:
			t.Fatal(err)
		}
	}
	if good != 1 || invalid != 1 {
		t.Fatalf("success=%d invalid=%d", good, invalid)
	}
	var user domain.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	if user.PasswordHash != "newhash" || user.SessionValidAfter == nil {
		t.Fatalf("password/boundary not committed: %+v", user.SessionValidAfter)
	}
	otherID := uuid.New()
	if err := db.Exec("INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, ?, ?, ?)", otherID, "other-"+otherID.String()+"@example.com", "unchanged", "Other", "User").Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id = ?", otherID)
	otherBoundary, err := store.SessionValidAfter(ctx, otherID)
	if err != nil || otherBoundary != nil {
		t.Fatalf("unaffected user's boundary=%v err=%v", otherBoundary, err)
	}
	// Issue and redeem a second reset before the first boundary's second has
	// elapsed. The later reset must revoke the intervening login token too.
	firstBoundary := *user.SessionValidAfter
	if err := store.Issue(ctx, userID, "again-hash", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume(ctx, "again-hash", "newerhash", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	if user.SessionValidAfter == nil || !user.SessionValidAfter.After(firstBoundary) {
		t.Fatalf("second reset did not advance boundary: first=%v second=%v", firstBoundary, user.SessionValidAfter)
	}
	if err := store.Issue(ctx, userID, "expired-hash", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume(ctx, "expired-hash", "bad", time.Now()); !errors.Is(err, domain.ErrInvalidResetToken) {
		t.Fatalf("expired token: %v", err)
	}
}
