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

func newMemberRepositoryWithMock(t *testing.T) (*memberRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &memberRepository{db: db}, mock
}

// newCapturingMemberRepository records the SQL the repository generates so tests can assert the
// predicates that enforce the permission invariants.
func newCapturingMemberRepository(t *testing.T, count int64) (*memberRepository, *string) {
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
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
	return &memberRepository{db: db}, captured
}

// TestHasActiveMemberPermissionAnswersFromScopedMembership is the repository-level proof of
// KEL-76 together with the ADR 0010 tenant scope: the count only matches a membership row of
// the caller in the operating tenant that still carries the role, and a role_permissions row
// whose role belongs to that tenant or is a system role.
func TestHasActiveMemberPermissionAnswersFromScopedMembership(t *testing.T) {
	tenantID, roleID, memberID, userID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name    string
		count   int64
		allowed bool
	}{
		{name: "allows when an active membership grants the permission", count: 1, allowed: true},
		{name: "denies when no active membership grants the permission", count: 0, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM tenant_members tm JOIN roles ro ON ro.id = tm.role_id JOIN role_permissions rp ON rp.role_id = ro.id JOIN permissions p ON p.id = rp.permission_id")).
				WithArgs(tenantID, roleID, true, "member:update", tenantID, memberID, userID).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.count))

			allowed, err := repo.HasActiveMemberPermission(context.Background(), domain.MemberPermissionQuery{
				TenantID: tenantID, RoleID: roleID, MemberID: memberID, UserID: userID, Permission: "member:update",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if allowed != test.allowed {
				t.Fatalf("allowed = %v, want %v", allowed, test.allowed)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet SQL expectations: %v", err)
			}
		})
	}
}

// TestHasActiveMemberPermissionReturnsDatabaseError keeps a failing database visible as an
// error instead of a silent denial or grant.
func TestHasActiveMemberPermissionReturnsDatabaseError(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	mock.ExpectQuery("SELECT count").WillReturnError(context.DeadlineExceeded)

	_, err := repo.HasActiveMemberPermission(context.Background(), domain.MemberPermissionQuery{
		TenantID: uuid.New(), RoleID: uuid.New(), UserID: uuid.New(), Permission: "member:update",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestHasActiveMemberPermissionQueryEnforcesInvariants asserts the generated SQL carries every
// predicate KEL-76 relies on, so none of them can silently regress.
func TestHasActiveMemberPermissionQueryEnforcesInvariants(t *testing.T) {
	repo, captured := newCapturingMemberRepository(t, 0)
	if _, err := repo.HasActiveMemberPermission(context.Background(), domain.MemberPermissionQuery{
		TenantID: uuid.New(), RoleID: uuid.New(), MemberID: uuid.New(), UserID: uuid.New(), Permission: "member:update",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, pattern := range []string{
		`tm\.tenant_id = \$\d+`,
		`tm\.role_id = \$\d+`,
		`tm\.is_active = \$\d+`,
		`tm\.deleted_at IS NULL`,
		`tm\.id = \$\d+`,
		`tm\.user_id = \$\d+`,
		`ro\.tenant_id = \$\d+ OR ro\.tenant_id IS NULL`,
	} {
		if !regexp.MustCompile(pattern).MatchString(*captured) {
			t.Errorf("SQL is missing %q: %s", pattern, *captured)
		}
	}
}

// TestHasActiveMemberPermissionWithoutMemberIDMatchesByUser covers tokens issued without the
// member_id claim: the lookup is keyed by the caller's user and never widened to any member.
func TestHasActiveMemberPermissionWithoutMemberIDMatchesByUser(t *testing.T) {
	repo, captured := newCapturingMemberRepository(t, 1)
	if _, err := repo.HasActiveMemberPermission(context.Background(), domain.MemberPermissionQuery{
		TenantID: uuid.New(), RoleID: uuid.New(), UserID: uuid.New(), Permission: "member:update",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(*captured, "tm.user_id =") {
		t.Fatalf("SQL is missing the user predicate: %s", *captured)
	}
	if strings.Contains(*captured, "tm.id =") {
		t.Fatalf("SQL pins a member id that was not supplied: %s", *captured)
	}
}

// TestHasActiveMemberPermissionDeniesIncompleteQueryWithoutLookup proves the fail-closed rule:
// a query that names no membership (or no tenant, role, or permission) is denied without SQL.
func TestHasActiveMemberPermissionDeniesIncompleteQueryWithoutLookup(t *testing.T) {
	full := domain.MemberPermissionQuery{TenantID: uuid.New(), RoleID: uuid.New(), MemberID: uuid.New(), UserID: uuid.New(), Permission: "member:update"}
	incomplete := map[string]domain.MemberPermissionQuery{
		"no member or user": {TenantID: full.TenantID, RoleID: full.RoleID, Permission: full.Permission},
		"no tenant":         {RoleID: full.RoleID, MemberID: full.MemberID, UserID: full.UserID, Permission: full.Permission},
		"no role":           {TenantID: full.TenantID, MemberID: full.MemberID, UserID: full.UserID, Permission: full.Permission},
		"no permission":     {TenantID: full.TenantID, RoleID: full.RoleID, MemberID: full.MemberID, UserID: full.UserID},
	}
	for name, query := range incomplete {
		t.Run(name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)
			allowed, err := repo.HasActiveMemberPermission(context.Background(), query)
			if err != nil || allowed {
				t.Fatalf("allowed = %v, err = %v, want denied without error", allowed, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected SQL: %v", err)
			}
		})
	}
}
