package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// KEL-81: DELETE /members/:id soft-deletes another member of the tenant and refuses the
// caller's own membership. Every rejection happens before the UPDATE that sets deleted_at, so
// the membership stays in place.

type memberDeleteFixture struct {
	tenant, member, user uuid.UUID
}

func newMemberDeleteFixture() memberDeleteFixture {
	return memberDeleteFixture{tenant: uuid.New(), member: uuid.New(), user: uuid.New()}
}

// expectMember asserts the lookup is tenant-scoped and skips soft-deleted rows.
func (f memberDeleteFixture) expectMember(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_members" WHERE (id = $1 AND tenant_id = $2) AND "tenant_members"."deleted_at" IS NULL`)).
		WithArgs(f.member, f.tenant, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "user_id", "role_id"}).AddRow(f.member, f.tenant, f.user, uuid.New()))
}

// expectSoftDelete asserts the write is a tenant-scoped soft delete, never a hard DELETE.
func (f memberDeleteFixture) expectSoftDelete(mock sqlmock.Sqlmock, rows int64) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_members" SET "deleted_at"=$1 WHERE (id = $2 AND tenant_id = $3) AND "tenant_members"."deleted_at" IS NULL`)).
		WithArgs(sqlmock.AnyArg(), f.member, f.tenant).
		WillReturnResult(sqlmock.NewResult(0, rows))
}

func TestDeleteMemberSoftDeletesAnotherMember(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newMemberDeleteFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	f.expectSoftDelete(mock, 1)
	mock.ExpectCommit()

	if err := repo.Delete(context.Background(), f.tenant, f.member, otherActor()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeleteMemberRejectsOwnMembershipBeforeAnyWrite covers the self-removal guard for tokens
// with a member_id claim and for legacy tokens identified only by user id.
func TestDeleteMemberRejectsOwnMembershipBeforeAnyWrite(t *testing.T) {
	f := newMemberDeleteFixture()
	actors := map[string]domain.Caller{
		"member_id claim":            {UserID: f.user, MemberID: f.member},
		"legacy token without claim": {UserID: f.user},
		"member_id matches only":     {UserID: uuid.New(), MemberID: f.member},
	}
	for name, actor := range actors {
		t.Run(name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)
			mock.ExpectBegin()
			f.expectMember(mock)
			mock.ExpectRollback()

			err := repo.Delete(context.Background(), f.tenant, f.member, actor)
			if !errors.Is(err, domain.ErrMemberSelfRemoval) {
				t.Fatalf("err=%v, want ErrMemberSelfRemoval", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected SQL (a write must not happen): %v", err)
			}
		})
	}
}

// TestDeleteMemberMissingOrForeignMemberIsNotFound: a member of another tenant, an unknown id
// and an already removed member all come back as no row from the scoped lookup.
func TestDeleteMemberMissingOrForeignMemberIsNotFound(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newMemberDeleteFixture()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_members"`)).
		WithArgs(f.member, f.tenant, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	if err := repo.Delete(context.Background(), f.tenant, f.member, otherActor()); !errors.Is(err, domain.ErrMemberNotFound) {
		t.Fatalf("err=%v, want ErrMemberNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestDeleteMemberConcurrentRemovalIsNotFound: when another admin removes the member between
// the lookup and the write, the soft delete affects no row and the second request gets 404.
func TestDeleteMemberConcurrentRemovalIsNotFound(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newMemberDeleteFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	f.expectSoftDelete(mock, 0)
	mock.ExpectRollback()

	if err := repo.Delete(context.Background(), f.tenant, f.member, otherActor()); !errors.Is(err, domain.ErrMemberNotFound) {
		t.Fatalf("err=%v, want ErrMemberNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMemberReturnsDatabaseError(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newMemberDeleteFixture()
	dbErr := errors.New("pq: connection reset")
	mock.ExpectBegin()
	f.expectMember(mock)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_members" SET "deleted_at"=$1`)).
		WithArgs(sqlmock.AnyArg(), f.member, f.tenant).
		WillReturnError(dbErr)
	mock.ExpectRollback()

	if err := repo.Delete(context.Background(), f.tenant, f.member, otherActor()); !errors.Is(err, dbErr) {
		t.Fatalf("err=%v, want %v", err, dbErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
