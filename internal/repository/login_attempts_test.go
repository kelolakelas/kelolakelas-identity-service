package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
)

func loginMock(t *testing.T) (*loginAttemptStore, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return NewLoginAttemptStore(db), mock
}

func TestLoginAttemptsLockoutExpiryResetAndUnknown(t *testing.T) {
	store, mock := loginMock(t)
	ctx := context.Background()
	id := uuid.New()
	passwordHash, _ := hash.HashPassword("right")
	columns := []string{"id", "email", "password_hash", "failed_login_attempts", "login_locked_until"}
	query := regexp.QuoteMeta(`SELECT * FROM "users" WHERE LOWER(email) = $1 AND "users"."deleted_at" IS NULL ORDER BY "users"."id" LIMIT $2 FOR UPDATE`)
	update := regexp.QuoteMeta(`UPDATE "users" SET`)
	for attempt := 0; attempt < 2; attempt++ {
		mock.ExpectBegin()
		mock.ExpectQuery(query).WithArgs("user@example.com", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(id, "User@example.com", passwordHash, attempt, nil))
		mock.ExpectExec(update).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if _, err := store.Authenticate(ctx, "USER@example.com", "wrong", passwordHash, 2, time.Minute); !errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	locked := time.Now().Add(time.Minute)
	mock.ExpectBegin()
	mock.ExpectQuery(query).WithArgs("user@example.com", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(id, "User@example.com", passwordHash, 0, locked))
	mock.ExpectRollback()
	if _, err := store.Authenticate(ctx, "USER@example.com", "right", passwordHash, 2, time.Minute); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("locked valid password: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(query).WithArgs("user@example.com", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(id, "User@example.com", passwordHash, 0, time.Now().Add(-time.Second)))
	mock.ExpectExec(update).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if user, err := store.Authenticate(ctx, "USER@example.com", "right", passwordHash, 2, time.Minute); err != nil || user.ID != id {
		t.Fatalf("expiry login: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(query).WithArgs("absent@example.com", 1).WillReturnRows(sqlmock.NewRows(columns))
	mock.ExpectRollback()
	if _, err := store.Authenticate(ctx, "ABSENT@example.com", "wrong", passwordHash, 2, time.Minute); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginAttemptDatabaseErrorDoesNotAuthenticate(t *testing.T) {
	store, mock := loginMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("db unavailable"))
	mock.ExpectRollback()
	if _, err := store.Authenticate(context.Background(), "a@example.com", "right", "dummy", 2, time.Minute); err == nil || errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("database failure swallowed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
