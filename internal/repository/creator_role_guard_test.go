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

func TestUpdateRoleCreatorDeniedWithinTransaction(t *testing.T) {
	for _, name := range []string{"creator caller", "teacher caller", "custom caller", "other tenant caller"} {
		t.Run(name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)
			tenant, member, target := uuid.New(), uuid.New(), uuid.New()
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"tenant_members\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "role_id"}).AddRow(member, tenant, uuid.New()))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"roles\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}).AddRow(target, nil, "Creator"))
			mock.ExpectRollback()
			result, err := repo.UpdateRole(context.Background(), tenant, member, target)
			if !errors.Is(err, domain.ErrCreatorGrantForbidden) || result != nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyCreatorInvitationCannotBeRedeemed(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	store := NewUserRepository(repo.db)
	token, role := uuid.New().String(), uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"tenant_invitations\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "token", "is_used", "expires_at"}).AddRow(uuid.New(), role, token, false, time.Now().Add(time.Hour)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"roles\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "tenant_id"}).AddRow(role, "Creator", nil))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM \"creator_requests\"")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	result, err := store.RegisterInvitedUserTx(context.Background(), token, "First", "Last", "password")
	if !errors.Is(err, domain.ErrCreatorGrantForbidden) || result != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
