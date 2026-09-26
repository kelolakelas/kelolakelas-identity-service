package middleware_test

import (
	"bytes"
	"encoding/json"
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

// openKEL79Database opens the isolated database named by KEL79_TEST_DATABASE_URL, with the
// identity migrations applied and seeders/ loaded (the system Creator and Teacher roles and
// their permissions), or skips like the other *_integration_test.go files.
func openKEL79Database(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("KEL79_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL79_TEST_DATABASE_URL for PostgreSQL integration test")
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

func systemRoleID(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	t.Helper()
	var raw string
	if err := db.Raw("SELECT id::text FROM roles WHERE name = ? AND tenant_id IS NULL", name).Scan(&raw).Error; err != nil || raw == "" {
		t.Fatalf("system role %s not seeded (run the seeders): err=%v", name, err)
	}
	return uuid.MustParse(raw)
}

type kel79Fixture struct {
	tenantA, tenantB                    uuid.UUID
	creatorRole, teacherRole            uuid.UUID
	managerRole, staffRole, foreignRole uuid.UUID
}

// seedKEL79 creates tenant A with a custom Manager role (member:update) and a custom Staff
// role (no permissions), and tenant B with its own Staff role.
func seedKEL79(t *testing.T, db *gorm.DB) kel79Fixture {
	t.Helper()
	suffix := uuid.NewString()[:8]
	f := kel79Fixture{
		tenantA: uuid.New(), tenantB: uuid.New(),
		creatorRole: systemRoleID(t, db, "Creator"), teacherRole: systemRoleID(t, db, "Teacher"),
		managerRole: uuid.New(), staffRole: uuid.New(), foreignRole: uuid.New(),
	}
	mustExec(t, db, "INSERT INTO tenants (id, name) VALUES (?, ?), (?, ?)", f.tenantA, "kel79-a-"+suffix, f.tenantB, "kel79-b-"+suffix)
	mustExec(t, db, "INSERT INTO roles (id, tenant_id, name) VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)",
		f.managerRole, f.tenantA, "Manager "+suffix,
		f.staffRole, f.tenantA, "Staff "+suffix,
		f.foreignRole, f.tenantB, "Staff "+suffix)
	mustExec(t, db, "INSERT INTO role_permissions (role_id, permission_id) SELECT ?, id FROM permissions WHERE name = 'member:update'", f.managerRole)
	t.Cleanup(func() {
		db.Exec("DELETE FROM tenant_members WHERE tenant_id IN (?, ?)", f.tenantA, f.tenantB)
		db.Exec("DELETE FROM role_permissions WHERE role_id IN (?, ?, ?)", f.managerRole, f.staffRole, f.foreignRole)
		db.Exec("DELETE FROM roles WHERE id IN (?, ?, ?)", f.managerRole, f.staffRole, f.foreignRole)
		db.Exec("DELETE FROM tenants WHERE id IN (?, ?)", f.tenantA, f.tenantB)
	})
	return f
}

type kel79Actor struct {
	kel76Member
	roleID uuid.UUID
}

// TestKEL79MemberRoleChangeMatrix drives PUT /members/:id/role and GET /members/:id through the
// real AuthMiddleware, MemberHandler, usecase and repository against PostgreSQL. Callers are
// Creator, Teacher and a custom role with member:update; targets are a Teacher member, the
// caller's own membership, and roles outside the tenant.
func TestKEL79MemberRoleChangeMatrix(t *testing.T) {
	db := openKEL79Database(t)
	f := seedKEL79(t, db)

	jwtService := jwt.NewJWTService("kel79-integration-secret-0123456789abcdef", time.Hour)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	memberHandler := handler.NewMemberHandler(usecase.NewMemberUsecase(repository.NewMemberRepository(db)))
	router.PUT("/api/v1/members/:id/role", middleware.AuthMiddleware(jwtService), memberHandler.UpdateMemberRole)
	router.GET("/api/v1/members/:id", middleware.AuthMiddleware(jwtService), memberHandler.GetMember)

	type envelope struct {
		Message string `json:"message"`
		Data    struct {
			Role struct {
				ID           uuid.UUID `json:"id"`
				IsSystemRole bool      `json:"is_system_role"`
			} `json:"role"`
		} `json:"data"`
	}
	do := func(t *testing.T, actor kel79Actor, memberIDClaim uuid.UUID, method, path, body string) (int, envelope) {
		t.Helper()
		token, err := jwtService.GenerateToken(actor.userID, "kel79@example.com", f.tenantA, actor.roleID, memberIDClaim, false)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var out envelope
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
		return rec.Code, out
	}
	changeRole := func(t *testing.T, actor kel79Actor, target, role uuid.UUID) (int, string) {
		t.Helper()
		code, out := do(t, actor, actor.memberID, http.MethodPut, "/api/v1/members/"+target.String()+"/role", `{"role_id":"`+role.String()+`"}`)
		return code, out.Message
	}
	currentRole := func(t *testing.T, memberID uuid.UUID) uuid.UUID {
		t.Helper()
		var raw string
		if err := db.Raw("SELECT role_id::text FROM tenant_members WHERE id = ?", memberID).Scan(&raw).Error; err != nil {
			t.Fatal(err)
		}
		return uuid.MustParse(raw)
	}
	newActor := func(t *testing.T, role uuid.UUID) kel79Actor {
		return kel79Actor{kel76Member: addMember(t, db, f.tenantA, role), roleID: role}
	}

	callers := []struct {
		name    string
		role    uuid.UUID
		allowed bool
	}{
		{"Creator caller", f.creatorRole, true},
		{"Teacher caller", f.teacherRole, false},
		{"custom caller with member:update", f.managerRole, true},
	}

	for _, caller := range callers {
		t.Run(caller.name, func(t *testing.T) {
			t.Run("moves a Teacher to a tenant custom role, visible in GET (AC1)", func(t *testing.T) {
				actor, teacher := newActor(t, caller.role), newActor(t, f.teacherRole)
				code, message := changeRole(t, actor, teacher.memberID, f.staffRole)
				if !caller.allowed {
					if code != http.StatusForbidden || currentRole(t, teacher.memberID) != f.teacherRole {
						t.Fatalf("status=%d %q, want 403 and unchanged role", code, message)
					}
					return
				}
				if code != http.StatusOK {
					t.Fatalf("status=%d %q, want 200", code, message)
				}
				getCode, got := do(t, actor, actor.memberID, http.MethodGet, "/api/v1/members/"+teacher.memberID.String(), "")
				if getCode != http.StatusOK || got.Data.Role.ID != f.staffRole || got.Data.Role.IsSystemRole {
					t.Fatalf("GET status=%d role=%+v, want Staff %s", getCode, got.Data.Role, f.staffRole)
				}
			})
			t.Run("own membership is a 409 and unchanged (AC2)", func(t *testing.T) {
				actor := newActor(t, caller.role)
				want, wantMessage := http.StatusConflict, "You cannot change your own member role"
				if !caller.allowed {
					want, wantMessage = http.StatusForbidden, "Permission denied"
				}
				for _, claim := range []uuid.UUID{actor.memberID, uuid.Nil} {
					code, out := do(t, actor, claim, http.MethodPut, "/api/v1/members/"+actor.memberID.String()+"/role", `{"role_id":"`+f.staffRole.String()+`"}`)
					if code != want || out.Message != wantMessage {
						t.Fatalf("member_id claim %s: status=%d %q, want %d %q", claim, code, out.Message, want, wantMessage)
					}
					if got := currentRole(t, actor.memberID); got != caller.role {
						t.Fatalf("role changed to %s", got)
					}
				}
			})
			t.Run("role of another tenant or unknown role is rejected (AC3)", func(t *testing.T) {
				actor, teacher := newActor(t, caller.role), newActor(t, f.teacherRole)
				want := http.StatusConflict
				if !caller.allowed {
					want = http.StatusForbidden
				}
				for _, role := range []uuid.UUID{f.foreignRole, uuid.New()} {
					if code, message := changeRole(t, actor, teacher.memberID, role); code != want {
						t.Fatalf("role %s: status=%d %q, want %d", role, code, message, want)
					}
				}
				if got := currentRole(t, teacher.memberID); got != f.teacherRole {
					t.Fatalf("role changed to %s", got)
				}
			})
			t.Run("system Creator is never granted (KEL-94)", func(t *testing.T) {
				actor, teacher := newActor(t, caller.role), newActor(t, f.teacherRole)
				code, message := changeRole(t, actor, teacher.memberID, f.creatorRole)
				if code != http.StatusForbidden || currentRole(t, teacher.memberID) != f.teacherRole {
					t.Fatalf("status=%d %q, want 403 and unchanged role", code, message)
				}
			})
			t.Run("a current Creator member keeps the guard", func(t *testing.T) {
				actor, creator := newActor(t, caller.role), newActor(t, f.creatorRole)
				code, message := changeRole(t, actor, creator.memberID, f.staffRole)
				wantMessage := "Creator role cannot be changed"
				if !caller.allowed {
					wantMessage = "Permission denied"
				}
				if code != http.StatusForbidden || message != wantMessage || currentRole(t, creator.memberID) != f.creatorRole {
					t.Fatalf("status=%d %q, want 403 %q and unchanged role", code, message, wantMessage)
				}
			})
		})
	}

	t.Run("custom-role member can move back to system Teacher", func(t *testing.T) {
		actor, staff := newActor(t, f.managerRole), newActor(t, f.staffRole)
		if code, message := changeRole(t, actor, staff.memberID, f.teacherRole); code != http.StatusOK || currentRole(t, staff.memberID) != f.teacherRole {
			t.Fatalf("status=%d %q, want 200 and Teacher", code, message)
		}
	})
	t.Run("member of another tenant is not found", func(t *testing.T) {
		actor := newActor(t, f.managerRole)
		foreign := addMember(t, db, f.tenantB, f.foreignRole)
		if code, message := changeRole(t, actor, foreign.memberID, f.staffRole); code != http.StatusNotFound || currentRole(t, foreign.memberID) != f.foreignRole {
			t.Fatalf("status=%d %q, want 404 and unchanged role", code, message)
		}
	})
}
