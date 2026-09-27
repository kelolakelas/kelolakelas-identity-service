package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func TestUserEmailConflictMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"case-insensitive index", &pgconn.PgError{Code: "23505", ConstraintName: "uq_users_email_lower"}, true},
		{"legacy exact constraint", &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}, true},
		{"other unique constraint", &pgconn.PgError{Code: "23505", ConstraintName: "uq_creator_requests_pending_target"}, false},
		{"other error class", &pgconn.PgError{Code: "23503", ConstraintName: "uq_users_email_lower"}, false},
		{"plain error", errors.New("boom"), false},
	} {
		if got := isUserEmailConflict(tc.err); got != tc.want {
			t.Fatalf("%s: got %v", tc.name, got)
		}
	}
}

func TestUserCreateStoresCanonicalEmailAndMapsIndexConflict(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewUserRepository(holder.db)
	user := &domain.User{ID: uuid.New(), Email: " New.User@Example.COM ", PasswordHash: "hash", FirstName: "A", LastName: "B"}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
		WithArgs("new.user@example.com", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), user.ID).
		WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_users_email_lower"})
	mock.ExpectRollback()
	if err := repo.Create(context.Background(), user); !errors.Is(err, domain.ErrUserAlreadyExists) {
		t.Fatalf("create conflict: %v", err)
	}
	if user.Email != "new.user@example.com" {
		t.Fatalf("user email %q", user.Email)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserGetByEmailComparesLowercase(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewUserRepository(holder.db)
	id := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`WHERE LOWER(email) = $1`)).
		WithArgs("legacy.user@example.com", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(id, "Legacy.User@Example.com"))
	got, err := repo.GetByEmail(context.Background(), "  LEGACY.user@example.COM ")
	if err != nil || got.ID != id {
		t.Fatalf("lookup: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInvitationLookupAndStoreUseCanonicalEmail(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewInvitationRepository(holder.db)
	tenant := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`LOWER(email) = $2`)).
		WithArgs(tenant, "member@example.com", false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	if _, err := repo.GetByTenantAndEmail(context.Background(), tenant, " Member@Example.COM"); err != nil {
		t.Fatal(err)
	}

	invitation := &domain.TenantInvitation{ID: uuid.New(), TenantID: tenant, RoleID: uuid.New(), Email: "Member@Example.COM", Token: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants"`)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(tenant))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_invitations"`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), tenant, "member@example.com", false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "tenant_invitations"`)).WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(time.Now(), time.Now()))
	mock.ExpectCommit()
	if err := repo.ReplaceActive(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	if invitation.Email != "member@example.com" {
		t.Fatalf("stored invitation email %q", invitation.Email)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
