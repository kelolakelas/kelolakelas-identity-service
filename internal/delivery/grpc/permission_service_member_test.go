package grpc

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestCheckPermissionWithMemberIDRequiresActiveMembership is the gRPC proof of KEL-76 AC3:
// with member_id the decision is answered from tenant_members, pinned to that membership in
// the request tenant with the request role, and only while it is active and not deleted.
func TestCheckPermissionWithMemberIDRequiresActiveMembership(t *testing.T) {
	tenantID, roleID, memberID := uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name    string
		count   int64
		allowed bool
	}{
		{name: "allows an active membership with the role", count: 1, allowed: true},
		{name: "denies a deleted, inactive, foreign-tenant or re-roled membership", count: 0, allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, mock := newTenantServiceServerWithMock(t)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM tenant_members tm JOIN roles ro ON ro.id = tm.role_id")).
				WithArgs(tenantID, roleID, true, "class:update", tenantID, memberID).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.count))

			response, err := server.CheckPermission(context.Background(), mustStruct(map[string]interface{}{
				"role_id":    roleID.String(),
				"permission": "class:update",
				"tenant_id":  tenantID.String(),
				"member_id":  memberID.String(),
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

// TestCheckPermissionWithMemberIDQueryEnforcesInvariants asserts the member_id SQL carries
// every membership predicate plus the ADR 0010 tenant-or-system-role scope.
func TestCheckPermissionWithMemberIDQueryEnforcesInvariants(t *testing.T) {
	captured := captureCheckPermissionSQL(t, map[string]interface{}{
		"role_id":    uuid.New().String(),
		"permission": "class:update",
		"tenant_id":  uuid.New().String(),
		"member_id":  uuid.New().String(),
	})
	for _, pattern := range []string{
		`tm\.id = \$\d+`,
		`tm\.tenant_id = \$\d+`,
		`tm\.role_id = \$\d+`,
		`tm\.is_active = \$\d+`,
		`tm\.deleted_at IS NULL`,
		`ro\.tenant_id = \$\d+ OR ro\.tenant_id IS NULL`,
	} {
		if !regexp.MustCompile(pattern).MatchString(captured) {
			t.Errorf("SQL is missing %q: %s", pattern, captured)
		}
	}
}

// TestCheckPermissionWithoutMemberIDKeepsLegacyQuery is the KEL-76 AC4 compatibility proof:
// an absent, null or empty member_id is answered by the unchanged role-only query.
func TestCheckPermissionWithoutMemberIDKeepsLegacyQuery(t *testing.T) {
	for name, memberValue := range map[string]interface{}{"absent": nil, "null": "null", "empty": ""} {
		t.Run(name, func(t *testing.T) {
			tenantID, roleID := uuid.New(), uuid.New()
			server, mock := newTenantServiceServerWithMock(t)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id JOIN roles r ON r.id = rp.role_id")).
				WithArgs(roleID, "class:update", tenantID).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

			fields := map[string]interface{}{
				"role_id":    roleID.String(),
				"permission": "class:update",
				"tenant_id":  tenantID.String(),
			}
			switch memberValue {
			case "null":
				fields["member_id"] = nil
			case "":
				fields["member_id"] = ""
			}
			response, err := server.CheckPermission(context.Background(), mustStruct(fields))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !response.GetFields()["allowed"].GetBoolValue() {
				t.Fatal("allowed = false, want true (legacy result)")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet SQL expectations: %v", err)
			}
		})
	}
}

// TestCheckPermissionRejectsInvalidMemberID covers the "member_id bukan UUID" edge case and
// the member_id-without-tenant rule: both are InvalidArgument and never reach the database.
func TestCheckPermissionRejectsInvalidMemberID(t *testing.T) {
	tenantID := uuid.New().String()
	tests := []struct {
		name   string
		fields map[string]interface{}
		want   string
	}{
		{name: "not a uuid", fields: map[string]interface{}{"tenant_id": tenantID, "member_id": "not-a-uuid"}, want: "member_id must be a UUID"},
		{name: "nil uuid", fields: map[string]interface{}{"tenant_id": tenantID, "member_id": uuid.Nil.String()}, want: "member_id must be a UUID"},
		{name: "not a string", fields: map[string]interface{}{"tenant_id": tenantID, "member_id": 42.0}, want: "member_id must be a UUID"},
		{name: "without tenant", fields: map[string]interface{}{"member_id": uuid.New().String()}, want: "tenant_id is required when member_id is set"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, mock := newTenantServiceServerWithMock(t)
			test.fields["role_id"] = uuid.New().String()
			test.fields["permission"] = "class:update"
			_, err := server.CheckPermission(context.Background(), mustStruct(test.fields))
			if status.Code(err) != codes.InvalidArgument || !contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want InvalidArgument containing %q", err, test.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected SQL: %v", err)
			}
		})
	}
}

// TestCheckPermissionWithMemberIDFailsClosedOnDatabaseError maps a lookup failure to the
// existing Internal error instead of allowing.
func TestCheckPermissionWithMemberIDFailsClosedOnDatabaseError(t *testing.T) {
	server, mock := newTenantServiceServerWithMock(t)
	mock.ExpectQuery("SELECT count").WillReturnError(errors.New("database unavailable"))

	response, err := server.CheckPermission(context.Background(), mustStruct(map[string]interface{}{
		"role_id":    uuid.New().String(),
		"permission": "class:update",
		"tenant_id":  uuid.New().String(),
		"member_id":  uuid.New().String(),
	}))
	if status.Code(err) != codes.Internal || response != nil {
		t.Fatalf("response = %v, error = %v, want Internal error", response, err)
	}
}
