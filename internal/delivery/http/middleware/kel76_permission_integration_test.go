package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/handler"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/middleware"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

// kel76Fixture is a small tenant graph seeded into a migrated PostgreSQL database.
type kel76Fixture struct {
	tenantA, tenantB                  uuid.UUID
	adminRole, editorRole, viewerRole uuid.UUID
	systemRole, foreignRole           uuid.UUID
}

type kel76Member struct {
	userID, memberID uuid.UUID
}

// openKEL76Database opens the isolated database named by KEL76_TEST_DATABASE_URL (identity
// migrations applied) or skips, like the other *_integration_test.go files.
func openKEL76Database(t *testing.T) *gorm.DB {
	t.Helper()
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
	return db
}

func mustExec(t *testing.T, db *gorm.DB, sql string, args ...interface{}) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// seedKEL76 creates two tenants and roles: admin/editor (member:delete) and viewer (none) in
// tenant A, a system role (tenant_id NULL) with member:delete, and an admin role in tenant B.
func seedKEL76(t *testing.T, db *gorm.DB) kel76Fixture {
	t.Helper()
	suffix := uuid.NewString()[:8]
	f := kel76Fixture{
		tenantA: uuid.New(), tenantB: uuid.New(),
		adminRole: uuid.New(), editorRole: uuid.New(), viewerRole: uuid.New(),
		systemRole: uuid.New(), foreignRole: uuid.New(),
	}
	mustExec(t, db, "INSERT INTO tenants (id, name) VALUES (?, ?), (?, ?)", f.tenantA, "kel76-a-"+suffix, f.tenantB, "kel76-b-"+suffix)
	mustExec(t, db, "INSERT INTO permissions (name) VALUES ('member:delete') ON CONFLICT (name) DO NOTHING")
	mustExec(t, db, "INSERT INTO roles (id, tenant_id, name) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?), (?, NULL, ?), (?, ?, ?)",
		f.adminRole, f.tenantA, "Admin "+suffix,
		f.editorRole, f.tenantA, "Editor "+suffix,
		f.viewerRole, f.tenantA, "Viewer "+suffix,
		f.systemRole, "System "+suffix,
		f.foreignRole, f.tenantB, "Admin "+suffix)
	for _, role := range []uuid.UUID{f.adminRole, f.editorRole, f.systemRole, f.foreignRole} {
		mustExec(t, db, "INSERT INTO role_permissions (role_id, permission_id) SELECT ?, id FROM permissions WHERE name = 'member:delete'", role)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM tenant_members WHERE tenant_id IN (?, ?)", f.tenantA, f.tenantB)
		db.Exec("DELETE FROM role_permissions WHERE role_id IN (?, ?, ?, ?, ?)", f.adminRole, f.editorRole, f.viewerRole, f.systemRole, f.foreignRole)
		db.Exec("DELETE FROM roles WHERE id IN (?, ?, ?, ?, ?)", f.adminRole, f.editorRole, f.viewerRole, f.systemRole, f.foreignRole)
		db.Exec("DELETE FROM tenants WHERE id IN (?, ?)", f.tenantA, f.tenantB)
	})
	return f
}

func addMember(t *testing.T, db *gorm.DB, tenantID, roleID uuid.UUID) kel76Member {
	t.Helper()
	m := kel76Member{userID: uuid.New(), memberID: uuid.New()}
	mustExec(t, db, "INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, 'x', 'K', 'Seventy')", m.userID, "kel76-"+m.userID.String()+"@example.com")
	mustExec(t, db, "INSERT INTO tenant_members (id, tenant_id, user_id, role_id) VALUES (?, ?, ?, ?)", m.memberID, tenantID, m.userID, roleID)
	t.Cleanup(func() {
		db.Exec("DELETE FROM tenant_members WHERE id = ?", m.memberID)
		db.Exec("DELETE FROM users WHERE id = ?", m.userID)
	})
	return m
}

// TestKEL76HTTPPermissionMembershipMatrix drives the real AuthMiddleware, MemberHandler,
// member usecase and repository against PostgreSQL. DELETE /members/:id is permission-gated
// (member:delete); targeting a non-existent member makes "allowed" observable as 404 without
// side effects, and "denied" as 403.
func TestKEL76HTTPPermissionMembershipMatrix(t *testing.T) {
	db := openKEL76Database(t)
	f := seedKEL76(t, db)

	jwtService := jwt.NewJWTService("kel76-integration-secret-0123456789abcdef", time.Hour)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	memberHandler := handler.NewMemberHandler(usecase.NewMemberUsecase(repository.NewMemberRepository(db)))
	router.DELETE("/api/v1/members/:id", middleware.AuthMiddleware(jwtService), memberHandler.DeleteMember)

	call := func(t *testing.T, userID, tenantID, roleID, memberID uuid.UUID) int {
		t.Helper()
		token, err := jwtService.GenerateToken(userID, "kel76@example.com", tenantID, roleID, memberID, false)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/members/"+uuid.NewString(), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	const allowed, denied = http.StatusNotFound, http.StatusForbidden

	t.Run("active member with the permission is allowed (AC5)", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		if got := call(t, m.userID, f.tenantA, f.adminRole, m.memberID); got != allowed {
			t.Fatalf("status = %d, want %d", got, allowed)
		}
	})
	t.Run("active member whose role lacks the permission is denied", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.viewerRole)
		if got := call(t, m.userID, f.tenantA, f.viewerRole, m.memberID); got != denied {
			t.Fatalf("status = %d, want %d", got, denied)
		}
	})
	t.Run("system role used by a tenant member is allowed", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.systemRole)
		if got := call(t, m.userID, f.tenantA, f.systemRole, m.memberID); got != allowed {
			t.Fatalf("status = %d, want %d", got, allowed)
		}
	})
	t.Run("soft-deleted member's old token is denied (AC1)", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		if got := call(t, m.userID, f.tenantA, f.adminRole, m.memberID); got != allowed {
			t.Fatalf("before delete: status = %d, want %d", got, allowed)
		}
		mustExec(t, db, "UPDATE tenant_members SET deleted_at = now() WHERE id = ?", m.memberID)
		if got := call(t, m.userID, f.tenantA, f.adminRole, m.memberID); got != denied {
			t.Fatalf("member_id token: status = %d, want %d", got, denied)
		}
		if got := call(t, m.userID, f.tenantA, f.adminRole, uuid.Nil); got != denied {
			t.Fatalf("legacy token: status = %d, want %d", got, denied)
		}
	})
	t.Run("inactive member is denied", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		mustExec(t, db, "UPDATE tenant_members SET is_active = false WHERE id = ?", m.memberID)
		if got := call(t, m.userID, f.tenantA, f.adminRole, m.memberID); got != denied {
			t.Fatalf("status = %d, want %d", got, denied)
		}
	})
	t.Run("token for another tenant than the membership is denied", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		if got := call(t, m.userID, f.tenantB, f.foreignRole, m.memberID); got != denied {
			t.Fatalf("member_id token: status = %d, want %d", got, denied)
		}
		if got := call(t, m.userID, f.tenantB, f.foreignRole, uuid.Nil); got != denied {
			t.Fatalf("legacy token: status = %d, want %d", got, denied)
		}
	})
	t.Run("role change denies the old token and re-login restores access (AC2)", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		mustExec(t, db, "UPDATE tenant_members SET role_id = ? WHERE id = ?", f.viewerRole, m.memberID)
		if got := call(t, m.userID, f.tenantA, f.adminRole, m.memberID); got != denied {
			t.Fatalf("old admin token after demotion: status = %d, want %d", got, denied)
		}
		if got := call(t, m.userID, f.tenantA, f.viewerRole, m.memberID); got != denied {
			t.Fatalf("new viewer token: status = %d, want %d", got, denied)
		}
		mustExec(t, db, "UPDATE tenant_members SET role_id = ? WHERE id = ?", f.editorRole, m.memberID)
		if got := call(t, m.userID, f.tenantA, f.viewerRole, m.memberID); got != denied {
			t.Fatalf("old viewer token after promotion: status = %d, want %d", got, denied)
		}
		if got := call(t, m.userID, f.tenantA, f.editorRole, m.memberID); got != allowed {
			t.Fatalf("new editor token: status = %d, want %d", got, allowed)
		}
	})
	t.Run("token pinned to an earlier membership id is denied", func(t *testing.T) {
		// tenant_members is UNIQUE (tenant_id, user_id), so a re-invited user gets a fresh
		// membership row id; a token still naming another membership id must not authorize it.
		m := addMember(t, db, f.tenantA, f.adminRole)
		if got := call(t, m.userID, f.tenantA, f.adminRole, uuid.New()); got != denied {
			t.Fatalf("status = %d, want %d", got, denied)
		}
	})
	t.Run("token of another user naming this membership is denied", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		if got := call(t, uuid.New(), f.tenantA, f.adminRole, m.memberID); got != denied {
			t.Fatalf("status = %d, want %d", got, denied)
		}
	})
	t.Run("legacy token without member_id matches the active membership by user", func(t *testing.T) {
		m := addMember(t, db, f.tenantA, f.adminRole)
		if got := call(t, m.userID, f.tenantA, f.adminRole, uuid.Nil); got != allowed {
			t.Fatalf("status = %d, want %d", got, allowed)
		}
	})
}
