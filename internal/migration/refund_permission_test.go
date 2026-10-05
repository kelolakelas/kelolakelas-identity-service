package migration

import (
	"os"
	"testing"
)

func TestKEL152RefundPermissionMigration(t *testing.T) {
	_, db, m := kel117Database(t)
	if err := m.Migrate(14); err != nil {
		t.Fatal(err)
	}
	kel117Seed(t, db)
	if err := db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'billing:refund'); DELETE FROM permissions WHERE name = 'billing:refund'").Error; err != nil {
		t.Fatal(err)
	}
	assert := func(permission, creator int64) {
		t.Helper()
		if n := kel117Count(t, db, "SELECT count(*) FROM permissions WHERE name = 'billing:refund'"); n != permission {
			t.Fatalf("permissions=%d", n)
		}
		if n := kel117Count(t, db, "SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id JOIN roles r ON r.id=rp.role_id WHERE p.name='billing:refund' AND r.name='Creator' AND r.tenant_id IS NULL"); n != creator {
			t.Fatalf("Creator grants=%d", n)
		}
		if n := kel117Count(t, db, "SELECT count(*) FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id JOIN roles r ON r.id=rp.role_id WHERE p.name='billing:refund' AND (r.name!='Creator' OR r.tenant_id IS NOT NULL)"); n != 0 {
			t.Fatalf("other grants=%d", n)
		}
	}
	if err := m.Steps(1); err != nil {
		t.Fatal(err)
	}
	assert(1, 1)
	sql, err := os.ReadFile("../../migrations/000015_billing_refund_permission.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(sql)).Error; err != nil {
		t.Fatal(err)
	}
	assert(1, 1)
	if err = m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assert(0, 0)
	if err = m.Steps(1); err != nil {
		t.Fatal(err)
	}
	kel117Seed(t, db)
	assert(1, 1)
}
