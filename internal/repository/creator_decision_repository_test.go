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

func TestCreatorDecisionRollbackOnAuthorizationReplayAndStaleTarget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		admin  bool
		status string
		target bool
		want   error
	}{
		{"revoked admin", false, "pending", false, domain.ErrCreatorRequestForbidden},
		{"replayed approval", true, "approved", false, domain.ErrCreatorRequestDecided},
		{"replayed rejection", true, "rejected", false, domain.ErrCreatorRequestDecided},
		{"stale target", true, "pending", true, domain.ErrCreatorRequestStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, mock := newMemberRepositoryWithMock(t)
			requestID, actorID, tenantID, targetID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
			mock.ExpectBegin()
			admins := sqlmock.NewRows([]string{"id"})
			if tc.admin {
				admins.AddRow(uuid.New())
			}
			mock.ExpectQuery("SELECT id FROM platform_admin_assignments").WillReturnRows(admins)
			if tc.admin {
				var target any
				if tc.target {
					target = targetID
				}
				mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"creator_requests\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "status", "target_email", "target_user_id"}).AddRow(requestID, tenantID, tc.status, "target@example.com", target))
			}
			if tc.target {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"tenants\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status"}).AddRow(tenantID, "School", "active"))
				mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"users\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "email"}))
			}
			mock.ExpectRollback()
			_, _, _, err := NewCreatorDecisionRepository(base.db).Decide(context.Background(), requestID, actorID, true, "")
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreatorDecisionRejectAtomicAudit(t *testing.T) {
	base, mock := newMemberRepositoryWithMock(t)
	requestID, actorID, tenantID := uuid.New(), uuid.New(), uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM platform_admin_assignments").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"creator_requests\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "status", "target_email"}).AddRow(requestID, tenantID, "pending", "new@example.com"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"tenants\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status"}).AddRow(tenantID, "School", "active"))
	mock.ExpectExec("UPDATE \"creator_requests\"").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO creator_request_audit").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	got, inv, _, err := NewCreatorDecisionRepository(base.db).Decide(context.Background(), requestID, actorID, false, "  reason  ")
	if err != nil || inv != nil || got.Status != "rejected" || *got.RejectionReason != "reason" || got.DecidedAt.After(time.Now()) {
		t.Fatalf("result=%+v invitation=%+v err=%v", got, inv, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
