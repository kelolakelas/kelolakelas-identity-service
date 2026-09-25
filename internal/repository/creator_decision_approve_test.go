package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestCreatorDecisionApproveExistingAndNewAccount(t *testing.T) {
	for _, existing := range []bool{true, false} {
		name := "new account"
		if existing {
			name = "existing account"
		}
		t.Run(name, func(t *testing.T) {
			base, mock := newMemberRepositoryWithMock(t)
			requestID, actorID, tenantID, targetID, roleID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			var target any
			if existing {
				target = targetID
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM platform_admin_assignments").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"creator_requests\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "status", "target_email", "target_user_id"}).AddRow(requestID, tenantID, "pending", "new@example.com", target))
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"tenants\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "status"}).AddRow(tenantID, "School", "active"))
			users := sqlmock.NewRows([]string{"id", "email", "is_parent"})
			if existing {
				users.AddRow(targetID, "new@example.com", false)
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"users\"")).WillReturnRows(users)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"roles\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "tenant_id"}).AddRow(roleID, "Creator", nil))
			if existing {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM \"tenant_members\"")).WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "user_id", "role_id", "is_active"}))
				mock.ExpectQuery("INSERT INTO \"tenant_members\"").WillReturnRows(sqlmock.NewRows([]string{"id", "joined_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
			} else {
				mock.ExpectQuery("INSERT INTO \"tenant_invitations\"").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(uuid.New(), time.Now(), time.Now()))
			}
			mock.ExpectExec("UPDATE \"creator_requests\"").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO creator_request_audit").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			result, invitation, _, err := NewCreatorDecisionRepository(base.db).Decide(context.Background(), requestID, actorID, true, "")
			if err != nil || result.Status != "approved" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if existing && (invitation != nil || result.TargetUserID == nil || *result.TargetUserID != targetID) {
				t.Fatalf("existing result=%+v invitation=%+v", result, invitation)
			}
			if !existing && (invitation == nil || invitation.Email != "new@example.com" || result.InvitationID == nil || result.TargetUserID != nil) {
				t.Fatalf("new result=%+v invitation=%+v", result, invitation)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
