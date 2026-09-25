package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestPasswordResetIssueStoresHashOnly(t *testing.T) {
	base, mock := newMemberRepositoryWithMock(t)
	store := NewPasswordResetRepository(base.db)
	id := uuid.New()
	rawToken := "opaque-reset-token"
	hash := "125e49a861546202dc256f709c3177d8517a24343886048331de339c48273e7e"
	if hash == rawToken {
		t.Fatal("raw token persisted")
	}
	expiry := time.Now().Add(time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users"`)).WithArgs(id, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).AddRow(id, "x@example.com", "stored"))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "password_reset_tokens"`)).WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "password_reset_tokens"`)).WithArgs(hash, id, expiry, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := store.Issue(context.Background(), id, hash, expiry); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
