package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func TestReplaceActiveInvitationsIsTransactional(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewInvitationRepository(holder.db)
	tenant := uuid.New()
	invitation := &domain.TenantInvitation{ID: uuid.New(), TenantID: tenant, RoleID: uuid.New(), Email: "member@example.com", Token: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants"`)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(tenant))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_invitations"`)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "tenant_invitations"`)).WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(time.Now(), time.Now()))
	mock.ExpectCommit()
	if err := repo.ReplaceActive(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceActiveRollsBackOnInsertFailure(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewInvitationRepository(holder.db)
	tenant := uuid.New()
	invitation := &domain.TenantInvitation{ID: uuid.New(), TenantID: tenant, RoleID: uuid.New(), Email: "member@example.com", Token: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour)}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenants"`)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(tenant))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_invitations"`)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "tenant_invitations"`)).WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	if err := repo.ReplaceActive(context.Background(), invitation); err == nil {
		t.Fatal("expected insert failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRevokeInvitationScopesTenantAndUnredeemed(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewInvitationRepository(holder.db)
	tenant, id := uuid.New(), uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_invitations"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	if err := repo.Revoke(context.Background(), tenant, id); !errors.Is(err, domain.ErrInvitationNotFound) {
		t.Fatalf("foreign invitation: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRevokedInvitationCannotRegister(t *testing.T) {
	holder, mock := newMemberRepositoryWithMock(t)
	repo := NewUserRepository(holder.db)
	token := uuid.NewString()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_invitations"`)).WillReturnRows(sqlmock.NewRows([]string{"id", "token", "is_used", "expires_at"}).AddRow(uuid.New(), token, true, time.Now().Add(time.Hour)))
	mock.ExpectRollback()
	user, err := repo.RegisterInvitedUserTx(context.Background(), token, "First", "Last", "password")
	if user != nil || !errors.Is(err, domain.ErrInvitationUsed) {
		t.Fatalf("registration: %v %v", user, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
