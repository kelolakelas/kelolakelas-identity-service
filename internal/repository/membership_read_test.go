package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func newMembershipReadMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, mock
}

func membershipRow(t *testing.T) *sqlmock.Rows {
	t.Helper()
	return sqlmock.NewRows([]string{"tenant_id", "member_id", "user_id", "role_id", "role_name"}).
		AddRow(uuid.New(), uuid.New(), uuid.New(), uuid.New(), "Teacher")
}

// TestFindActiveMembershipScopesByActiveMembershipAndRole pins the SQL the KEL-136
// read generates: the same active-membership predicates the permission check relies
// on (KEL-76, ADR 0010 tenant scope), plus the member/user pinning of the caller.
func TestFindActiveMembershipScopesByActiveMembershipAndRole(t *testing.T) {
	tenantID, roleID, memberID, userID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	db, mock := newMembershipReadMock(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT tm.tenant_id, tm.id AS member_id, tm.user_id, tm.role_id, ro.name AS role_name FROM tenant_members tm JOIN roles ro ON ro.id = tm.role_id")).
		WithArgs(tenantID, roleID, true, tenantID, memberID, userID).
		WillReturnRows(membershipRow(t))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT p.name FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id")).
		WillReturnRows(sqlmock.NewRows([]string{"name"}).
			AddRow("attendance:read").
			AddRow("schedule:read"))

	membership, err := FindActiveMembership(context.Background(), db, tenantID, roleID, memberID, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if membership.RoleName != "Teacher" {
		t.Fatalf("role name = %q", membership.RoleName)
	}
	if len(membership.PermissionNames) != 2 || membership.PermissionNames[0] != "attendance:read" {
		t.Fatalf("permissions = %v", membership.PermissionNames)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

// TestFindActiveMembershipReturnsInactiveWithoutRow proves an unknown, inactive,
// or re-roled membership surfaces as ErrMembershipInactive instead of an empty
// 200-shaped answer.
func TestFindActiveMembershipReturnsInactiveWithoutRow(t *testing.T) {
	db, mock := newMembershipReadMock(t)
	mock.ExpectQuery("SELECT tm.tenant_id").
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "member_id", "user_id", "role_id", "role_name"}))

	membership, err := FindActiveMembership(context.Background(), db, uuid.New(), uuid.New(), uuid.New(), uuid.New())
	if err != domain.ErrMembershipInactive {
		t.Fatalf("error = %v, want ErrMembershipInactive", err)
	}
	if membership != nil {
		t.Fatalf("membership = %+v, want nil", membership)
	}
}

// TestFindActiveMembershipReturnsDatabaseError keeps a failing database visible.
func TestFindActiveMembershipReturnsDatabaseError(t *testing.T) {
	db, mock := newMembershipReadMock(t)
	mock.ExpectQuery("SELECT tm.tenant_id").WillReturnError(context.DeadlineExceeded)

	if _, err := FindActiveMembership(context.Background(), db, uuid.New(), uuid.New(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected error")
	}
}

// newCapturingMembershipRead opens a gorm handle that records every SQL statement
// it executes and answers an empty membership row, so the invariant test can run
// FindActiveMembership and inspect the generated query.
func newCapturingMembershipRead(t *testing.T) (*gorm.DB, *string) {
	t.Helper()
	captured := new(string)
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(_, actualSQL string) error {
			*captured = actualSQL
			return nil
		},
	)))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "member_id", "user_id", "role_id", "role_name"}))
	return db, captured
}

// TestFindActiveMembershipQueryEnforcesInvariants asserts the generated SQL carries
// every predicate the KEL-136 read relies on, so none of them can silently regress.
func TestFindActiveMembershipQueryEnforcesInvariants(t *testing.T) {
	cases := []struct {
		name     string
		memberID uuid.UUID
	}{
		{name: "token with member_id pins the membership row", memberID: uuid.New()},
		{name: "legacy token matches by user only", memberID: uuid.Nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			db, captured := newCapturingMembershipRead(t)
			if _, err := FindActiveMembership(context.Background(), db, uuid.New(), uuid.New(), testCase.memberID, uuid.New()); err == nil {
				t.Fatal("expected ErrMembershipInactive for the empty row")
			}

			for _, pattern := range []string{
				`tm\.tenant_id = \$\d+`,
				`tm\.role_id = \$\d+`,
				`tm\.is_active = \$\d+`,
				`tm\.deleted_at IS NULL`,
				`ro\.tenant_id = \$\d+ OR ro\.tenant_id IS NULL`,
				`tm\.user_id = \$\d+`,
			} {
				if !regexp.MustCompile(pattern).MatchString(*captured) {
					t.Errorf("SQL is missing %q: %s", pattern, *captured)
				}
			}
			if testCase.memberID != uuid.Nil && !strings.Contains(*captured, "tm.id =") {
				t.Errorf("SQL is missing the member_id pin: %s", *captured)
			}
			if testCase.memberID == uuid.Nil && strings.Contains(*captured, "tm.id =") {
				t.Errorf("SQL pins a member id that was not supplied: %s", *captured)
			}
		})
	}
}
