package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func TestPlatformCreatorRequestListPendingProjection(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	id, tenant, requester := uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT cr.id, cr.tenant_id, t.name AS tenant_name, cr.requester_user_id, cr.target_email, cr.target_user_id, cr.reason, cr.status, cr.created_at, cr.updated_at, cr.decided_by, cr.decided_at, cr.rejection_reason FROM creator_requests AS cr JOIN tenants AS t ON t.id = cr.tenant_id WHERE cr.status = $1 ORDER BY cr.created_at ASC, cr.id ASC`)).
		WithArgs("pending").WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "tenant_name", "requester_user_id", "target_email", "reason", "status", "created_at", "updated_at"}).AddRow(id, tenant, "Example", requester, "target@example.com", "reason", "pending", time.Now(), time.Now()))
	items, err := NewPlatformCreatorRequestRepository(repo.db).ListPlatform(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TenantName != "Example" || items[0].TenantID != tenant || items[0].ID != id {
		t.Fatalf("items=%+v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
