package repository

import (
	"context"
	"crypto/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformFactorPostgresSingleUseAndRecovery(t *testing.T) {
	dsn := os.Getenv("KEL105_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL105_TEST_DATABASE_URL for isolated migrated database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	id := uuid.New()
	ctx := context.Background()
	if err := db.Exec("INSERT INTO users (id,email,password_hash,first_name,last_name) VALUES (?,?,?,?,?)", id, id.String()+"@example.test", "hash", "Admin", "Test").Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Exec("DELETE FROM platform_factor_challenges WHERE user_id = ?", id)
		db.Exec("DELETE FROM platform_admin_assignments WHERE user_id = ?", id)
		db.Exec("DELETE FROM users WHERE id = ?", id)
	}()
	if err := db.Exec("INSERT INTO platform_admin_assignments(user_id,enrollment_allowed,factor_version) VALUES (?,true,1)", id).Error; err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	rand.Read(key)
	store := NewPlatformFactorStore(db, key)
	token := make([]byte, 32)
	rand.Read(token)
	secret := []byte("only-in-encrypted-column")
	if _, err := store.Begin(ctx, id, "enroll", token, secret, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	wrong, err := store.Begin(ctx, id, "enroll", []byte("other-32-byte-token-not-really!!"), secret, time.Now().Add(time.Minute))
	if err == nil {
		t.Log("second challenge accepted")
	}
	_ = wrong
	var raw []byte
	if err := db.Raw("SELECT pending_secret FROM platform_factor_challenges WHERE user_id = ?", id).Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) == string(secret) {
		t.Fatal("secret persisted in plaintext")
	}
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := store.Consume(ctx, id, "enroll", token, time.Now(), func(got []byte) (bool, error) { return string(got) == string(secret), nil })
			if err != nil {
				t.Errorf("consume: %v", err)
			}
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted=%d", accepted)
	}
	if version, err := store.Version(ctx, id); err != nil || version != 2 {
		t.Fatalf("version=%d err=%v", version, err)
	}
	if err := db.Exec("UPDATE platform_admin_assignments SET factor_secret=NULL,enrollment_allowed=true,factor_version=factor_version+1 WHERE user_id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := store.Version(ctx, id); err == nil {
		t.Fatal("recovered factor remained valid")
	}
}
