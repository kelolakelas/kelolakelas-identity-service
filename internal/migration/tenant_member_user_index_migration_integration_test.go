package migration

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

const (
	kel59PreVersion   = 11
	kel59IndexVersion = 12
	kel59Index        = "idx_tenant_members_user_id"
	kel59IndexDef     = "CREATE INDEX idx_tenant_members_user_id ON public.tenant_members USING btree (user_id)"
)

// kel59Database creates an empty, uniquely named database and returns its DSN.
// Set KEL59_TEST_ADMIN_DATABASE_URL to a PostgreSQL URL whose role may
// CREATE/DROP DATABASE; every run gets its own database so migration state
// never leaks between runs.
func kel59Database(t *testing.T) (string, *gorm.DB) {
	t.Helper()
	admin := os.Getenv("KEL59_TEST_ADMIN_DATABASE_URL")
	if admin == "" {
		t.Skip("set KEL59_TEST_ADMIN_DATABASE_URL for the migration 000012 PostgreSQL test")
	}
	adminDB, err := gorm.Open(postgres.Open(admin), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	name := "kel59_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return dsn, db
}

func kel59Migrator(t *testing.T, dsn string) *migrate.Migrate {
	t.Helper()
	dir, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New((&url.URL{Scheme: "file", Path: dir}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Migrate(kel59PreVersion); err != nil {
		t.Fatalf("migrate to %d: %v", kel59PreVersion, err)
	}
	return m
}

type kel59IndexState struct {
	IsUnique bool
	IsValid  bool
	Def      string
}

func kel59Lookup(t *testing.T, db *gorm.DB) *kel59IndexState {
	t.Helper()
	var rows []kel59IndexState
	err := db.Raw(`SELECT i.indisunique AS is_unique, i.indisvalid AS is_valid, pg_get_indexdef(i.indexrelid) AS def
		FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = ?`, kel59Index).Scan(&rows).Error
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

// sqlCapture records every statement GORM executes, with bound values inlined,
// so the test EXPLAINs exactly the SQL the repository sends rather than a copy.
type sqlCapture struct {
	logger.Interface
	mu   sync.Mutex
	sqls []string
}

func (c *sqlCapture) LogMode(logger.LogLevel) logger.Interface { return c }

func (c *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func (c *sqlCapture) take(t *testing.T, contains ...string) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, sql := range c.sqls {
		match := true
		for _, want := range contains {
			match = match && strings.Contains(sql, want)
		}
		if match {
			out = append(out, sql)
		}
	}
	c.sqls = nil
	if len(out) != 1 {
		t.Fatalf("want exactly one captured statement containing %q, got %q", contains, out)
	}
	return out[0]
}

// kel59Plan runs EXPLAIN on sql and returns the text plan plus every index the
// plan reads.
func kel59Plan(t *testing.T, db *gorm.DB, sql string) (string, map[string]bool) {
	t.Helper()
	var lines []string
	if err := db.Raw("EXPLAIN " + sql).Scan(&lines).Error; err != nil {
		t.Fatalf("explain %s: %v", sql, err)
	}
	var raw string
	if err := db.Raw("EXPLAIN (FORMAT JSON) " + sql).Row().Scan(&raw); err != nil {
		t.Fatalf("explain json %s: %v", sql, err)
	}
	var plans []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("parse plan %q: %v", raw, err)
	}
	indexes := map[string]bool{}
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if name, ok := node["Index Name"].(string); ok {
			indexes[name] = true
		}
		children, _ := node["Plans"].([]any)
		for _, child := range children {
			if m, ok := child.(map[string]any); ok {
				walk(m)
			}
		}
	}
	walk(plans[0].Plan)
	return strings.Join(lines, "\n"), indexes
}

// KEL-59: migration 000012 adds a non-unique tenant_members(user_id) index, the
// active-membership lookup by user uses it with an unchanged result, and down
// removes only that index.
func TestMigration000012TenantMemberUserIndexUpExplainDown(t *testing.T) {
	dsn, db := kel59Database(t)
	m := kel59Migrator(t, dsn)
	ctx := context.Background()

	// 6,000 users, each a member of 2 of 10 tenants (12,000 memberships), with a
	// quarter of the memberships inactive. The unique (tenant_id, user_id)
	// constraint leads with tenant_id, so it cannot serve a user-only lookup.
	seed := []string{
		`INSERT INTO roles (id, tenant_id, name) VALUES ('00000000-0000-0000-0000-00000000a059', NULL, 'kel59-member')`,
		`INSERT INTO tenants (id, name) SELECT md5('kel59-tenant-' || g)::uuid, 'kel59 tenant ' || g FROM generate_series(0, 9) AS g`,
		`INSERT INTO users (id, email, password_hash, first_name, last_name)
			SELECT md5('kel59-user-' || g)::uuid, 'kel59-' || g || '@example.com', 'x', 'KEL', '59' FROM generate_series(1, 6000) AS g`,
		`INSERT INTO tenant_members (tenant_id, user_id, role_id, is_active, joined_at)
			SELECT md5('kel59-tenant-' || ((g + k * 5) % 10))::uuid, md5('kel59-user-' || g)::uuid,
				'00000000-0000-0000-0000-00000000a059', NOT (k = 0 AND g % 2 = 0),
				timestamp '2026-09-01' + (g * 2 + k) * interval '1 minute'
			FROM generate_series(1, 6000) AS g, generate_series(0, 1) AS k`,
	}
	for _, sql := range seed {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	var userText string
	if err := db.Raw(`SELECT md5('kel59-user-42')::uuid::text`).Row().Scan(&userText); err != nil {
		t.Fatal(err)
	}
	user := uuid.MustParse(userText)

	capture := &sqlCapture{Interface: logger.Discard}
	repo := repository.NewUserRepository(db.Session(&gorm.Session{Logger: capture}))
	lookup := func() (uuid.UUID, string) {
		t.Helper()
		member, err := repo.GetTenantMemberByUserID(ctx, user)
		if err != nil || member == nil {
			t.Fatalf("GetTenantMemberByUserID: member=%v err=%v", member, err)
		}
		if member.UserID != user || !member.IsActive {
			t.Fatalf("lookup returned %+v", member)
		}
		return member.ID, capture.take(t, `"tenant_members"`, "user_id")
	}
	analyze := func() {
		t.Helper()
		if err := db.Exec("ANALYZE tenant_members").Error; err != nil {
			t.Fatal(err)
		}
	}

	analyze()
	beforeID, beforeSQL := lookup()
	plan, _ := kel59Plan(t, db, beforeSQL)
	t.Logf("BEFORE %d\n%s\n%s", kel59IndexVersion, beforeSQL, plan)

	if err := m.Steps(1); err != nil {
		t.Fatalf("up: %v", err)
	}
	if version, dirty, err := m.Version(); err != nil || version != kel59IndexVersion || dirty {
		t.Fatalf("version after up = %d dirty=%v err=%v", version, dirty, err)
	}
	state := kel59Lookup(t, db)
	if state == nil || state.IsUnique || !state.IsValid || state.Def != kel59IndexDef {
		t.Fatalf("index after up = %+v, want a valid non-unique %q", state, kel59IndexDef)
	}

	analyze()
	afterID, afterSQL := lookup()
	if afterID != beforeID || afterSQL != beforeSQL {
		t.Fatalf("lookup changed: before=%s %q after=%s %q", beforeID, beforeSQL, afterID, afterSQL)
	}
	plan, indexes := kel59Plan(t, db, afterSQL)
	t.Logf("AFTER %d\n%s\n%s", kel59IndexVersion, afterSQL, plan)
	if !indexes[kel59Index] {
		t.Fatalf("plan does not use %s:\n%s", kel59Index, plan)
	}

	// A user may still join a further tenant (non-unique on user_id).
	if err := db.Exec(`INSERT INTO tenant_members (tenant_id, user_id, role_id)
		VALUES (md5('kel59-tenant-3')::uuid, ?, '00000000-0000-0000-0000-00000000a059')`, user).Error; err != nil {
		t.Fatalf("third membership for one user rejected: %v", err)
	}

	if err := m.Steps(-1); err != nil {
		t.Fatalf("down: %v", err)
	}
	if version, dirty, err := m.Version(); err != nil || version != kel59PreVersion || dirty {
		t.Fatalf("version after down = %d dirty=%v err=%v", version, dirty, err)
	}
	if state := kel59Lookup(t, db); state != nil {
		t.Fatalf("%s still present after down: %+v", kel59Index, state)
	}
	var rows int64
	if err := db.Raw(`SELECT count(*) FROM tenant_members`).Scan(&rows).Error; err != nil || rows != 12001 {
		t.Fatalf("down changed data: rows=%d err=%v", rows, err)
	}
	for _, other := range []string{"uq_tenant_members_tenant_user", "idx_tenant_members_deleted_at", "uq_users_email_lower"} {
		var n int64
		if err := db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = ?`, other).Scan(&n).Error; err != nil || n != 1 {
			t.Fatalf("down touched %s: count=%d err=%v", other, n, err)
		}
	}

	// Up tolerates an index that an operator already created by hand.
	if err := db.Exec(`CREATE INDEX ` + kel59Index + ` ON tenant_members (user_id)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(1); err != nil {
		t.Fatalf("up over a pre-existing index: %v", err)
	}
	if state := kel59Lookup(t, db); state == nil || state.Def != kel59IndexDef {
		t.Fatalf("index after re-apply = %+v", state)
	}
}
