package migration

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	permissiongrpc "github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/grpc"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

// Each scenario owns a database. The admin URL must allow CREATE/DROP DATABASE.
func kel117Database(t *testing.T) (string, *gorm.DB, *migrate.Migrate) {
	t.Helper()
	admin := os.Getenv("KEL117_TEST_ADMIN_DATABASE_URL")
	if admin == "" {
		t.Skip("set KEL117_TEST_ADMIN_DATABASE_URL for PostgreSQL migration test")
	}
	adminDB, err := gorm.Open(postgres.Open(admin), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	name := "kel117_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := adminDB.Exec("CREATE DATABASE " + name).Error; err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New((&url.URL{Scheme: "file", Path: dir}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = m.Close()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := m.Migrate(13); err != nil {
		t.Fatalf("migrate to 13: %v", err)
	}
	return dsn, db, m
}

func kel117Seed(t *testing.T, db *gorm.DB) {
	t.Helper()
	contents, err := os.ReadFile("../../seeders/000001_default_permissions_and_roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(contents)).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func kel117Count(t *testing.T, db *gorm.DB, query string) int64 {
	t.Helper()
	var count int64
	if err := db.Raw(query).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func kel117AssertGrants(t *testing.T, db *gorm.DB, wantPermission, wantCreator int64) {
	t.Helper()
	if got := kel117Count(t, db, `SELECT count(*) FROM permissions WHERE name = 'chat:manage'`); got != wantPermission {
		t.Fatalf("chat:manage rows = %d, want %d", got, wantPermission)
	}
	if got := kel117Count(t, db, `SELECT count(*) FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id JOIN roles r ON r.id = rp.role_id
		WHERE p.name = 'chat:manage' AND r.name = 'Creator' AND r.tenant_id IS NULL`); got != wantCreator {
		t.Fatalf("Creator grants = %d, want %d", got, wantCreator)
	}
	if got := kel117Count(t, db, `SELECT count(*) FROM role_permissions rp
		JOIN permissions p ON p.id = rp.permission_id JOIN roles r ON r.id = rp.role_id
		WHERE p.name = 'chat:manage' AND (r.name != 'Creator' OR r.tenant_id IS NOT NULL)`); got != 0 {
		t.Fatalf("unexpected non-Creator grants = %d", got)
	}
}

func TestMigration000014ChatManageSeedAndPermission(t *testing.T) {
	_, db, m := kel117Database(t)
	kel117Seed(t, db) // production: system roles already exist before the new migration
	if err := m.Steps(1); err != nil {
		t.Fatalf("up: %v", err)
	}
	kel117AssertGrants(t, db, 1, 1)
	// GET /api/v1/permissions uses this repository's catalog query.
	catalog, err := repository.NewRbacRepository(db).GetPermissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, permission := range catalog {
		if permission.Name == "chat:manage" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("catalog has %d chat:manage entries, want 1", found)
	}

	tenant := uuid.New()
	if err := db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", tenant, "kel117-"+tenant.String()).Error; err != nil {
		t.Fatal(err)
	}
	server := permissiongrpc.NewTenantServiceServer(db)
	for _, tc := range []struct {
		role string
		want bool
	}{{"Creator", true}, {"Teacher", false}} {
		t.Run(tc.role, func(t *testing.T) {
			var roleID uuid.UUID
			if err := db.Raw("SELECT id FROM roles WHERE name = ? AND tenant_id IS NULL", tc.role).Row().Scan(&roleID); err != nil {
				t.Fatal(err)
			}
			userID, memberID := uuid.New(), uuid.New()
			if err := db.Exec("INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES (?, ?, 'x', 'KEL', '117')", userID, "kel117-"+userID.String()+"@example.com").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("INSERT INTO tenant_members (id, tenant_id, user_id, role_id) VALUES (?, ?, ?, ?)", memberID, tenant, userID, roleID).Error; err != nil {
				t.Fatal(err)
			}
			request, err := structpb.NewStruct(map[string]interface{}{
				"tenant_id": tenant.String(), "role_id": roleID.String(), "member_id": memberID.String(), "permission": "chat:manage",
			})
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.CheckPermission(context.Background(), request)
			if err != nil || response.GetFields()["allowed"].GetBoolValue() != tc.want {
				t.Fatalf("CheckPermission = %v, %v; want allowed=%v", response, err, tc.want)
			}
		})
	}
	invalid, _ := structpb.NewStruct(map[string]interface{}{"tenant_id": tenant.String(), "role_id": "invalid", "permission": "chat:manage"})
	if _, err := server.CheckPermission(context.Background(), invalid); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("malformed role: %v, want InvalidArgument", err)
	}

	// Replay the SQL directly: golang-migrate skips an already-applied version.
	up, err := os.ReadFile("../../migrations/000014_chat_manage_permission.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(up)).Error; err != nil {
		t.Fatalf("replay up: %v", err)
	}
	kel117Seed(t, db)
	kel117AssertGrants(t, db, 1, 1)
	if err := m.Steps(-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	kel117AssertGrants(t, db, 0, 0)
	if err := m.Steps(1); err != nil {
		t.Fatalf("up after down: %v", err)
	}
	kel117AssertGrants(t, db, 1, 1)
}

func TestMigration000014ChatManageBeforeSeed(t *testing.T) {
	_, db, m := kel117Database(t)
	if err := m.Steps(1); err != nil {
		t.Fatalf("up without roles: %v", err)
	}
	kel117AssertGrants(t, db, 1, 0)
	kel117Seed(t, db)
	kel117AssertGrants(t, db, 1, 1)
	kel117Seed(t, db)
	kel117AssertGrants(t, db, 1, 1)
}

func TestMigration000014ChatManageExistingCatalogAndTenantCreator(t *testing.T) {
	_, db, m := kel117Database(t)
	kel117Seed(t, db)
	tenantID, tenantCreatorID := uuid.New(), uuid.New()
	var permissionID uuid.UUID
	if err := db.Raw("UPDATE permissions SET description = 'Pre-existing' WHERE name = 'chat:manage' RETURNING id").Row().Scan(&permissionID); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, "kel117-"+tenantID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO roles (id, tenant_id, name) VALUES (?, ?, 'Creator')", tenantCreatorID, tenantID).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("up with existing catalog: %v", err)
	}
	kel117AssertGrants(t, db, 1, 1)
	var gotID uuid.UUID
	var description string
	if err := db.Raw("SELECT id, description FROM permissions WHERE name = 'chat:manage'").Row().Scan(&gotID, &description); err != nil {
		t.Fatal(err)
	}
	if gotID != permissionID || description != "Pre-existing" {
		t.Fatalf("up rewrote existing permission: id=%s description=%q", gotID, description)
	}
}
