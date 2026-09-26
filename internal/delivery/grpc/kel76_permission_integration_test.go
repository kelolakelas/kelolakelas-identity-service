package grpc

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestKEL76CheckPermissionMembershipMatrix runs CheckPermission against a migrated PostgreSQL
// database (KEL76_TEST_DATABASE_URL). With member_id the answer follows the membership row
// (AC3). Without member_id it matches the pre-KEL-76 role-only answer (AC4).
func TestKEL76CheckPermissionMembershipMatrix(t *testing.T) {
	dsn := os.Getenv("KEL76_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL76_TEST_DATABASE_URL for PostgreSQL integration test")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	exec := func(sql string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	suffix := uuid.NewString()[:8]
	tenantA, tenantB := uuid.New(), uuid.New()
	adminRole, viewerRole, systemRole := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO tenants (id, name) VALUES (?, ?), (?, ?)", tenantA, "kel76g-a-"+suffix, tenantB, "kel76g-b-"+suffix)
	exec("INSERT INTO permissions (name) VALUES ('class:update') ON CONFLICT (name) DO NOTHING")
	exec("INSERT INTO roles (id, tenant_id, name) VALUES (?, ?, ?), (?, ?, ?), (?, NULL, ?)",
		adminRole, tenantA, "Admin "+suffix, viewerRole, tenantA, "Viewer "+suffix, systemRole, "System "+suffix)
	for _, role := range []uuid.UUID{adminRole, systemRole} {
		exec("INSERT INTO role_permissions (role_id, permission_id) SELECT ?, id FROM permissions WHERE name = 'class:update'", role)
	}
	var memberIDs, userIDs []uuid.UUID
	addMember := func(tenantID, roleID uuid.UUID, update string) uuid.UUID {
		userID, memberID := uuid.New(), uuid.New()
		exec("INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, 'x', 'K', 'Seventy')", userID, "kel76g-"+userID.String()+"@example.com")
		exec("INSERT INTO tenant_members (id, tenant_id, user_id, role_id) VALUES (?, ?, ?, ?)", memberID, tenantID, userID, roleID)
		if update != "" {
			exec("UPDATE tenant_members SET "+update+" WHERE id = ?", memberID)
		}
		memberIDs = append(memberIDs, memberID)
		userIDs = append(userIDs, userID)
		return memberID
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tenant_members WHERE id IN ?", memberIDs)
		db.Exec("DELETE FROM users WHERE id IN ?", userIDs)
		db.Exec("DELETE FROM role_permissions WHERE role_id IN (?, ?, ?)", adminRole, viewerRole, systemRole)
		db.Exec("DELETE FROM roles WHERE id IN (?, ?, ?)", adminRole, viewerRole, systemRole)
		db.Exec("DELETE FROM tenants WHERE id IN (?, ?)", tenantA, tenantB)
	})

	active := addMember(tenantA, adminRole, "")
	systemMember := addMember(tenantA, systemRole, "")
	deleted := addMember(tenantA, adminRole, "deleted_at = now()")
	inactive := addMember(tenantA, adminRole, "is_active = false")
	reRoled := addMember(tenantA, viewerRole, "")

	server := &TenantServiceServer{db: db}
	check := func(t *testing.T, fields map[string]interface{}) bool {
		t.Helper()
		fields["permission"] = "class:update"
		response, err := server.CheckPermission(context.Background(), mustStruct(fields))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return response.GetFields()["allowed"].GetBoolValue()
	}

	cases := []struct {
		name   string
		tenant uuid.UUID
		role   uuid.UUID
		member uuid.UUID
		want   bool
	}{
		{name: "active membership", tenant: tenantA, role: adminRole, member: active, want: true},
		{name: "system role membership", tenant: tenantA, role: systemRole, member: systemMember, want: true},
		{name: "soft-deleted membership", tenant: tenantA, role: adminRole, member: deleted, want: false},
		{name: "inactive membership", tenant: tenantA, role: adminRole, member: inactive, want: false},
		{name: "different tenant", tenant: tenantB, role: adminRole, member: active, want: false},
		{name: "different role (membership re-roled)", tenant: tenantA, role: adminRole, member: reRoled, want: false},
		{name: "unknown membership id", tenant: tenantA, role: adminRole, member: uuid.New(), want: false},
	}
	for _, c := range cases {
		t.Run("member_id/"+c.name, func(t *testing.T) {
			got := check(t, map[string]interface{}{"role_id": c.role.String(), "tenant_id": c.tenant.String(), "member_id": c.member.String()})
			if got != c.want {
				t.Fatalf("allowed = %v, want %v", got, c.want)
			}
		})
	}

	// AC4: without member_id the result is the role-only answer, whatever the membership state.
	legacy := []struct {
		name   string
		fields map[string]interface{}
		want   bool
	}{
		{name: "tenant role with permission", fields: map[string]interface{}{"role_id": adminRole.String(), "tenant_id": tenantA.String()}, want: true},
		{name: "system role", fields: map[string]interface{}{"role_id": systemRole.String(), "tenant_id": tenantA.String()}, want: true},
		{name: "role without permission", fields: map[string]interface{}{"role_id": viewerRole.String(), "tenant_id": tenantA.String()}, want: false},
		{name: "role from another tenant", fields: map[string]interface{}{"role_id": adminRole.String(), "tenant_id": tenantB.String()}, want: false},
		{name: "no tenant_id (transition)", fields: map[string]interface{}{"role_id": adminRole.String()}, want: true},
	}
	for _, c := range legacy {
		t.Run("without member_id/"+c.name, func(t *testing.T) {
			if got := check(t, c.fields); got != c.want {
				t.Fatalf("allowed = %v, want %v", got, c.want)
			}
		})
	}
}
