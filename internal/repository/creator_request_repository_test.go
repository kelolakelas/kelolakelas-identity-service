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

func expectCreator(mock sqlmock.Sqlmock, id uuid.UUID) {
	rows := sqlmock.NewRows([]string{"id"})
	if id != uuid.Nil {
		rows.AddRow(id)
	}
	mock.ExpectQuery("SELECT tm.id FROM tenant_members tm").WillReturnRows(rows)
}

func TestCreatorRequestLiveMembershipAndTenantIsolation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int64
		want  error
	}{
		{"active Creator in tenant", 1, nil},
		{"Teacher", 0, domain.ErrCreatorRequestForbidden},
		{"custom role", 0, domain.ErrCreatorRequestForbidden},
		{"other tenant Creator", 0, domain.ErrCreatorRequestForbidden},
		{"inactive Creator", 0, domain.ErrCreatorRequestForbidden},
		{"removed Creator", 0, domain.ErrCreatorRequestForbidden},
		{"stale Creator token", 0, domain.ErrCreatorRequestForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)
			store := NewCreatorRequestRepository(repo.db)
			tenant, user, role := uuid.New(), uuid.New(), uuid.New()
			mock.ExpectBegin()
			memberID := uuid.Nil
			if tc.count == 1 {
				memberID = uuid.New()
			}
			expectCreator(mock, memberID)
			if tc.count == 1 {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"creator_requests\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "requester_user_id", "target_email", "reason", "status", "created_at", "updated_at"}).AddRow(uuid.New(), tenant, user, "a@example.com", "why", "pending", time.Now(), time.Now()))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			items, err := store.List(context.Background(), tenant, user, role)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if tc.count == 1 && len(items) != 1 {
				t.Fatalf("items=%v", items)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreatorRequestCreatePendingDoesNotAssignRoleAndRejectsDuplicate(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		repo, mock := newMemberRepositoryWithMock(t)
		store := NewCreatorRequestRepository(repo.db)
		tenant, user, role := uuid.New(), uuid.New(), uuid.New()
		mock.ExpectBegin()
		expectCreator(mock, uuid.New())
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"users\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "email"}))
		insert := mock.ExpectExec("INSERT INTO \"creator_requests\"")
		if duplicate {
			insert.WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "uq_creator_requests_pending_target"})
			mock.ExpectRollback()
		} else {
			insert.WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		created, err := store.Create(context.Background(), tenant, user, role, "a@example.com", nil, "reason")
		if duplicate {
			if !errors.Is(err, domain.ErrCreatorRequestDuplicate) || created != nil {
				t.Fatalf("duplicate: %v %+v", err, created)
			}
		} else if err != nil || created.Status != "pending" || created.TenantID != tenant || created.TargetEmail != "a@example.com" {
			t.Fatalf("request: %v %+v", err, created)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreatorRequestTargetAlreadyCreatorRejectedBeforeInsert(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	tenant, user, role, target := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mock.ExpectBegin()
	expectCreator(mock, uuid.New())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"users\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).AddRow(target, "target@example.com"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM tenant_members tm JOIN roles ro ON ro.id = tm.role_id").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()
	_, err := NewCreatorRequestRepository(repo.db).Create(context.Background(), tenant, user, role, "target@example.com", &target, "reason")
	if !errors.Is(err, domain.ErrCreatorTargetAlreadyCreator) {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
