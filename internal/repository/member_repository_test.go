package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

// TestHasPermissionScopesRoleToTenantOrSystemRole is the repository-level proof of acceptance
// criterion 2: the permission lookup only counts role_permissions rows whose role belongs to
// the operating tenant or is a system role (tenant_id IS NULL). A role owned by another tenant
// is therefore never able to satisfy a permission check.
func TestHasPermissionScopesRoleToTenantOrSystemRole(t *testing.T) {
	tenantID := uuid.New()
	roleID := uuid.New()

	tests := []struct {
		name    string
		count   int64
		allowed bool
	}{
		{name: "allows when a matching row exists in scope", count: 1, allowed: true},
		{name: "denies when no row is in scope", count: 0, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)

			// The SQL must constrain the role by the operating tenant, while still accepting
			// system roles through the tenant_id IS NULL branch.
			mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id JOIN roles ro ON ro.id = rp.role_id")).
				WithArgs(roleID, "member:update", tenantID).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.count))

			allowed, err := repo.HasPermission(context.Background(), tenantID, roleID, "member:update")
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

// TestHasPermissionCountsRowsReturnedByScopedQuery captures DML errors so a failing database
// surfaces as an error instead of a silent denial.
func TestHasPermissionCountsRowsReturnedByScopedQuery(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	wantErr := context.DeadlineExceeded

	mock.ExpectQuery("SELECT count").WillReturnError(wantErr)

	if _, err := repo.HasPermission(context.Background(), uuid.New(), uuid.New(), "member:update"); err == nil {
		t.Fatal("expected error")
	}
}

// TestHasPermissionQueryMentionsTenantScope asserts the generated SQL contains the
// tenant-or-system-role predicate so the invariant cannot silently regress.
func TestHasPermissionQueryMentionsTenantScope(t *testing.T) {
	var capturedSQL string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(expectedSQL, actualSQL string) error {
			capturedSQL = actualSQL
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

	mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	repo := &memberRepository{db: db}
	if _, err := repo.HasPermission(context.Background(), uuid.New(), uuid.New(), "member:update"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !regexp.MustCompile(`ro\.tenant_id\s*=\s*\$?\d*\s*OR\s+ro\.tenant_id IS NULL`).MatchString(capturedSQL) {
		t.Fatalf("SQL is missing the tenant-or-system-role scope: %s", capturedSQL)
	}
}
