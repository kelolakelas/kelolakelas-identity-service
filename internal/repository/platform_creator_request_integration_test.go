package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformCreatorRequestPostgresCrossTenantPendingOnly(t *testing.T) {
	dsn := os.Getenv("KEL103_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL103_TEST_DATABASE_URL for isolated migrated database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	requester := uuid.New()
	if err := db.Exec("INSERT INTO users(id,email,password_hash,first_name,last_name) VALUES(?,?,'hash','Test','User')", requester, requester.String()+"@example.com").Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE id = ?", requester)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for i, tenant := range ids {
		if err := db.Exec("INSERT INTO tenants(id,name) VALUES(?,?)", tenant, "kel103-"+tenant.String()).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Exec("DELETE FROM tenants WHERE id = ?", tenant)
		status := "pending"
		if i == 1 {
			status = "approved"
		}
		if err := db.Exec("INSERT INTO creator_requests(id,tenant_id,requester_user_id,target_email,reason,status) VALUES(?,?,?,?,?,?)", uuid.New(), tenant, requester, "target-"+tenant.String()+"@example.com", "Reason", status).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Exec("DELETE FROM creator_requests WHERE tenant_id = ?", tenant)
	}
	items, err := NewPlatformCreatorRequestRepository(db).ListPlatform(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TenantID != ids[0] || items[0].TenantName != "kel103-"+ids[0].String() || items[0].Status != "pending" {
		t.Fatalf("queue=%+v", items)
	}
}
