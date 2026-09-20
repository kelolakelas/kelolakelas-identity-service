package grpc

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTenantServiceServerWithMock builds the gRPC server on top of a mocked database so the
// tests can observe the SQL that backs an authorization decision.
func newTenantServiceServerWithMock(t *testing.T) (*TenantServiceServer, sqlmock.Sqlmock) {
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
	return &TenantServiceServer{db: db}, mock
}

// TestCheckPermissionValidatesTenantContract covers the tenant_id validation added for
// KEL-20: a malformed tenant_id is rejected on the request seam, exactly like role_id.
func TestCheckPermissionValidatesTenantContract(t *testing.T) {
	server := &TenantServiceServer{}
	req := mustStruct(map[string]interface{}{
		"role_id":    uuid.New().String(),
		"permission": "class:update",
		"tenant_id":  "not-a-uuid",
	})

	_, err := server.CheckPermission(context.Background(), req)
	if status.Code(err) != codes.InvalidArgument || !contains(err.Error(), "tenant_id must be a UUID") {
		t.Fatalf("error=%v, want InvalidArgument containing %q", err, "tenant_id must be a UUID")
	}
}

// TestCheckPermissionRejectsMissingTenantWhenRequired proves the second half of the ADR 0002
// rollout: once every caller sends tenant_id, PERMISSION_REQUIRE_TENANT_ID makes identity
// reject requests that omit it instead of silently falling back to a global lookup.
func TestCheckPermissionRejectsMissingTenantWhenRequired(t *testing.T) {
	requirePermissionTenantID(t, true)

	server := &TenantServiceServer{}
	req := mustStruct(map[string]interface{}{
		"role_id":    uuid.New().String(),
		"permission": "class:update",
	})

	_, err := server.CheckPermission(context.Background(), req)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("error=%v, want InvalidArgument", err)
	}
	if !contains(err.Error(), "tenant_id is required") {
		t.Fatalf("error=%v, want tenant_id is required", err)
	}
}

// TestCheckPermissionScopesDecisionToTenant is the gRPC-level proof of acceptance criteria 2
// and 3: a request carrying tenant_id must be answered by a query that only counts roles
// belonging to that tenant or to the system (tenant_id IS NULL). A role lifted from another
// tenant, or a role deleted after the token was issued, therefore resolves to no rows and is
// denied.
func TestCheckPermissionScopesDecisionToTenant(t *testing.T) {
	tenantID := uuid.New()
	roleID := uuid.New()

	tests := []struct {
		name    string
		count   int64
		allowed bool
	}{
		{name: "allows a role in scope", count: 1, allowed: true},
		{name: "denies a foreign or deleted role", count: 0, allowed: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, mock := newTenantServiceServerWithMock(t)

			mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id JOIN roles r ON r.id = rp.role_id")).
				WithArgs(roleID, "class:update", tenantID).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.count))

			response, err := server.CheckPermission(context.Background(), mustStruct(map[string]interface{}{
				"role_id":    roleID.String(),
				"permission": "class:update",
				"tenant_id":  tenantID.String(),
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := response.GetFields()["allowed"].GetBoolValue(); got != test.allowed {
				t.Fatalf("allowed = %v, want %v", got, test.allowed)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet SQL expectations: %v", err)
			}
		})
	}
}

// TestCheckPermissionScopedQueryHasTenantPredicate asserts the generated SQL really carries
// the tenant-or-system-role predicate, so the invariant cannot silently regress.
func TestCheckPermissionScopedQueryHasTenantPredicate(t *testing.T) {
	captured := captureCheckPermissionSQL(t, map[string]interface{}{
		"role_id":    uuid.New().String(),
		"permission": "class:update",
		"tenant_id":  uuid.New().String(),
	})

	if !regexp.MustCompile(`r\.tenant_id\s*=\s*\$\d+\s*OR\s+r\.tenant_id IS NULL`).MatchString(captured) {
		t.Fatalf("SQL is missing the tenant-or-system-role scope: %s", captured)
	}
}

// TestCheckPermissionAcceptsMissingTenantDuringTransition documents the deploy order in ADR
// 0002: identity is deployed before academic, so it must keep answering a request that still
// sends only role_id and permission. During the transition window the legacy unscoped lookup
// is used and no tenant predicate may appear.
func TestCheckPermissionAcceptsMissingTenantDuringTransition(t *testing.T) {
	requirePermissionTenantID(t, false)

	captured := captureCheckPermissionSQL(t, map[string]interface{}{
		"role_id":    uuid.New().String(),
		"permission": "class:update",
	})

	if strings.Contains(captured, "tenant_id") {
		t.Fatalf("legacy request was scoped during the transition window: %s", captured)
	}
}

// captureCheckPermissionSQL runs CheckPermission against a mocked database and returns the SQL
// that was executed. The lookup is answered with "denied" because only the query shape matters.
func captureCheckPermissionSQL(t *testing.T, fields map[string]interface{}) string {
	t.Helper()

	var captured string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(_, actualSQL string) error {
			captured = actualSQL
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

	server := &TenantServiceServer{db: db}
	response, err := server.CheckPermission(context.Background(), mustStruct(fields))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if response.GetFields()["allowed"].GetBoolValue() {
		t.Fatal("expected the lookup to deny")
	}
	return captured
}

// requirePermissionTenantID flips the package-level switch and restores it afterwards so tests
// stay independent of each other.
func requirePermissionTenantID(t *testing.T, required bool) {
	t.Helper()
	previous := RequirePermissionTenantID
	RequirePermissionTenantID = required
	t.Cleanup(func() { RequirePermissionTenantID = previous })
}
