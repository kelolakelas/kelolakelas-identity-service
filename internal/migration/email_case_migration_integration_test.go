package migration

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
)

const kel89PreVersion = 10

// kel89Database creates an empty, uniquely named database and returns its DSN.
// Set KEL89_TEST_ADMIN_DATABASE_URL to a PostgreSQL URL whose role may
// CREATE/DROP DATABASE; every scenario gets its own database so migration
// state (including a dirty version) never leaks between runs.
func kel89Database(t *testing.T) (string, *gorm.DB) {
	t.Helper()
	admin := os.Getenv("KEL89_TEST_ADMIN_DATABASE_URL")
	if admin == "" {
		t.Skip("set KEL89_TEST_ADMIN_DATABASE_URL for the migration 000011 PostgreSQL test")
	}
	adminDB, err := gorm.Open(postgres.Open(admin), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	name := "kel89_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := adminDB.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return dsn, db
}

func kel89Migrate(t *testing.T, dsn string) *migrate.Migrate {
	t.Helper()
	dir, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New((&url.URL{Scheme: "file", Path: dir}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Migrate(kel89PreVersion); err != nil {
		t.Fatalf("migrate to %d: %v", kel89PreVersion, err)
	}
	return m
}

func kel89InsertUser(t *testing.T, db *gorm.DB, email, passwordHash string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec(`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, ?, 'Legacy', 'User')`, id, email, passwordHash).Error; err != nil {
		t.Fatalf("insert %s: %v", email, err)
	}
	return id
}

func kel89IndexExists(t *testing.T, db *gorm.DB) bool {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT count(*) FROM pg_indexes WHERE tablename = 'users' AND indexname = 'uq_users_email_lower'`).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestMigration000011CleanDataUpLegacyLoginAndRollback(t *testing.T) {
	dsn, db := kel89Database(t)
	m := kel89Migrate(t, dsn)
	passwordHash, err := hash.HashPassword("correct")
	if err != nil {
		t.Fatal(err)
	}
	legacyID := kel89InsertUser(t, db, "Legacy.User@Example.com", passwordHash)
	kel89InsertUser(t, db, "other@example.com", passwordHash)

	if err := m.Steps(1); err != nil {
		t.Fatalf("up on clean data: %v", err)
	}
	if version, dirty, err := m.Version(); err != nil || version != 11 || dirty {
		t.Fatalf("version after up = %d dirty=%v err=%v", version, dirty, err)
	}
	if !kel89IndexExists(t, db) {
		t.Fatal("uq_users_email_lower missing after up")
	}
	var stored string
	if err := db.Raw(`SELECT email FROM users WHERE id = ?`, legacyID).Scan(&stored).Error; err != nil || stored != "Legacy.User@Example.com" {
		t.Fatalf("migration rewrote legacy email to %q (%v)", stored, err)
	}

	// Legacy mixed-case account logs in with any letter case after migration.
	ctx := context.Background()
	store := repository.NewLoginAttemptStore(db)
	for _, email := range []string{"legacy.user@example.com", "LEGACY.USER@EXAMPLE.COM"} {
		user, err := store.Authenticate(ctx, domain.NormalizeEmail(email), "correct", passwordHash, 5, time.Minute)
		if err != nil || user.ID != legacyID {
			t.Fatalf("legacy login %q: %v", email, err)
		}
	}
	users := repository.NewUserRepository(db)
	if got, err := users.GetByEmail(ctx, "LEGACY.user@EXAMPLE.com"); err != nil || got.ID != legacyID {
		t.Fatalf("lookup legacy: %v", err)
	}

	// The index rejects a case-only duplicate even when the app-level lookup
	// is bypassed, and the repository reports it as ErrUserAlreadyExists.
	err = db.Exec(`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, 'LEGACY.USER@example.com', 'x', 'A', 'B')`, uuid.New()).Error
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_users_email_lower" {
		t.Fatalf("raw case-variant insert: %v", err)
	}
	dup := &domain.User{ID: uuid.New(), Email: "legacy.user@example.com", PasswordHash: "x", FirstName: "A", LastName: "B"}
	if err := users.Create(ctx, dup); !errors.Is(err, domain.ErrUserAlreadyExists) {
		t.Fatalf("repository case-variant create: %v", err)
	}
	fresh := &domain.User{ID: uuid.New(), Email: " Fresh@Example.COM ", PasswordHash: "x", FirstName: "A", LastName: "B"}
	if err := users.Create(ctx, fresh); err != nil {
		t.Fatalf("fresh create: %v", err)
	}
	if err := db.Raw(`SELECT email FROM users WHERE id = ?`, fresh.ID).Scan(&stored).Error; err != nil || stored != "fresh@example.com" {
		t.Fatalf("fresh stored as %q (%v)", stored, err)
	}

	if err := m.Steps(-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if version, dirty, err := m.Version(); err != nil || version != kel89PreVersion || dirty {
		t.Fatalf("version after down = %d dirty=%v err=%v", version, dirty, err)
	}
	if kel89IndexExists(t, db) {
		t.Fatal("uq_users_email_lower still present after down")
	}
	var legacyRows int64
	if err := db.Raw(`SELECT count(*) FROM users`).Scan(&legacyRows).Error; err != nil || legacyRows != 3 {
		t.Fatalf("rollback changed data: rows=%d err=%v", legacyRows, err)
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("re-apply after rollback: %v", err)
	}
	if !kel89IndexExists(t, db) {
		t.Fatal("index missing after re-apply")
	}
}

func TestMigration000011StopsOnCaseConflictWithoutChangingData(t *testing.T) {
	dsn, db := kel89Database(t)
	m := kel89Migrate(t, dsn)
	first := kel89InsertUser(t, db, "dup@example.com", "hash")
	second := kel89InsertUser(t, db, "DUP@Example.com", "hash")
	kel89InsertUser(t, db, "unique@example.com", "hash")

	err := m.Steps(1)
	if err == nil {
		t.Fatal("migration applied despite case-insensitive duplicate emails")
	}
	msg := err.Error()
	for _, want := range []string{"email case conflict", "1 email address(es)", first.String(), second.String()} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not mention %q", msg, want)
		}
	}
	if strings.Contains(strings.ToLower(msg), "dup@example.com") {
		t.Fatalf("error leaks the conflicting address: %q", msg)
	}
	if kel89IndexExists(t, db) {
		t.Fatal("index created despite conflict")
	}
	var emails []string
	if err := db.Raw(`SELECT email FROM users WHERE id IN (?, ?) ORDER BY email`, first, second).Scan(&emails).Error; err != nil {
		t.Fatal(err)
	}
	if len(emails) != 2 || emails[0] != "DUP@Example.com" || emails[1] != "dup@example.com" {
		t.Fatalf("conflicting rows changed: %v", emails)
	}
	if version, dirty, err := m.Version(); err != nil || version != 11 || !dirty {
		t.Fatalf("expected golang-migrate to mark 11 dirty, got %d dirty=%v err=%v", version, dirty, err)
	}

	// Documented recovery: resolve the accounts manually, force the previous
	// version, and apply again.
	if err := db.Exec(`DELETE FROM users WHERE id = ?`, second).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Force(kel89PreVersion); err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("up after manual resolution: %v", err)
	}
	if !kel89IndexExists(t, db) {
		t.Fatal("index missing after recovery")
	}
}
