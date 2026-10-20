package repository

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// KEL181_TEST_DATABASE_URL must point at a disposable PostgreSQL database; the
// real migration chain is applied before the creator decision scenarios run.
// This is the regression suite for the admin-assignment primary key: the table
// is keyed by user_id and has no id column, so a decision that selects id
// fails against real PostgreSQL with SQLSTATE 42703.
func TestKEL181CreatorDecisionPostgres(t *testing.T) {
	dsn := os.Getenv("KEL181_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL181_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	dir, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New((&url.URL{Scheme: "file", Path: dir}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()

	// The repository resolves the global Creator role with First (ordered by
	// primary key); reuse any existing row so the pick stays deterministic.
	// Seeded environments ship the global Creator role, but a database with
	// only the migration chain applied does not; tolerate the missing row and
	// insert it below so the suite is self-contained.
	var creatorRoleID uuid.UUID
	err = db.Raw("SELECT id FROM roles WHERE name = 'Creator' AND tenant_id IS NULL ORDER BY id LIMIT 1").Row().Scan(&creatorRoleID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if creatorRoleID == uuid.Nil {
		creatorRoleID = uuid.New()
		if err := db.Exec("INSERT INTO roles(id,name) VALUES (?,'Creator')", creatorRoleID).Error; err != nil {
			t.Fatal(err)
		}
		defer db.Exec("DELETE FROM roles WHERE id = ?", creatorRoleID)
	}
	isCreatorRole := func(t *testing.T, roleID uuid.UUID) bool {
		t.Helper()
		var ok bool
		if err := db.Raw("SELECT EXISTS (SELECT 1 FROM roles WHERE id = ? AND name = 'Creator' AND tenant_id IS NULL)", roleID).Row().Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}

	// A fixture tracks every row one subtest creates and removes them in an
	// FK-safe order: audit rows, requests, invitations/members per tenant,
	// then assignments and users.
	type fixture struct {
		requests []uuid.UUID
		tenants  []uuid.UUID
		users    []uuid.UUID
	}
	newFixture := func(t *testing.T) *fixture {
		f := &fixture{}
		t.Cleanup(func() {
			for _, id := range f.requests {
				db.Exec("DELETE FROM creator_request_audit WHERE request_id = ?", id)
				db.Exec("DELETE FROM creator_requests WHERE id = ?", id)
			}
			for _, id := range f.tenants {
				db.Exec("DELETE FROM tenant_invitations WHERE tenant_id = ?", id)
				db.Exec("DELETE FROM tenant_members WHERE tenant_id = ?", id)
				db.Exec("DELETE FROM tenants WHERE id = ?", id)
			}
			for _, id := range f.users {
				db.Exec("DELETE FROM platform_admin_assignments WHERE user_id = ?", id)
				db.Exec("DELETE FROM tenant_members WHERE user_id = ?", id)
				db.Exec("DELETE FROM users WHERE id = ?", id)
			}
		})
		return f
	}
	user := func(t *testing.T, f *fixture, isParent bool) (uuid.UUID, string) {
		t.Helper()
		id := uuid.New()
		email := id.String() + "@kel181.test"
		if err := db.Exec("INSERT INTO users(id,email,password_hash,first_name,last_name,is_parent) VALUES (?,?,'hash','Kel','OneEightOne',?)", id, email, isParent).Error; err != nil {
			t.Fatal(err)
		}
		f.users = append(f.users, id)
		return id, email
	}
	admin := func(t *testing.T, f *fixture, active bool) uuid.UUID {
		t.Helper()
		id, _ := user(t, f, false)
		if err := db.Exec("INSERT INTO platform_admin_assignments(user_id,is_active) VALUES (?,?)", id, active).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	tenant := func(t *testing.T, f *fixture, status string) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := db.Exec("INSERT INTO tenants(id,name,status) VALUES (?,?,?)", id, "kel181-"+id.String(), status).Error; err != nil {
			t.Fatal(err)
		}
		f.tenants = append(f.tenants, id)
		return id
	}
	request := func(t *testing.T, f *fixture, tenantID, requesterID uuid.UUID, email string, targetUserID *uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		var target any
		if targetUserID != nil {
			target = *targetUserID
		}
		if err := db.Exec("INSERT INTO creator_requests(id,tenant_id,requester_user_id,target_email,target_user_id,reason,status) VALUES (?,?,?,?,?,'kel181 reason','pending')", id, tenantID, requesterID, email, target).Error; err != nil {
			t.Fatal(err)
		}
		f.requests = append(f.requests, id)
		return id
	}
	repo := NewCreatorDecisionRepository(db)
	decide := func(actorID, requestID uuid.UUID, approve bool, reason string) (*domain.CreatorRequest, error) {
		req, _, _, err := repo.Decide(context.Background(), requestID, actorID, approve, reason)
		return req, err
	}
	type requestState struct {
		Status          string
		DecidedBy       *uuid.UUID
		RejectionReason *string
		InvitationID    *uuid.UUID
		TargetUserID    *uuid.UUID
	}
	loadRequest := func(t *testing.T, id uuid.UUID) requestState {
		t.Helper()
		var state requestState
		if err := db.Raw("SELECT status, decided_by, rejection_reason, invitation_id, target_user_id FROM creator_requests WHERE id = ?", id).Scan(&state).Error; err != nil {
			t.Fatal(err)
		}
		return state
	}
	count := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("approve existing target once then replay is decided", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		target, email := user(t, f, false)
		requestID := request(t, f, tenantID, requester, email, &target)
		got, err := decide(actor, requestID, true, "")
		if err != nil || got == nil || got.Status != "approved" {
			t.Fatalf("approve: result=%+v err=%v", got, err)
		}
		state := loadRequest(t, requestID)
		if state.Status != "approved" || state.DecidedBy == nil || *state.DecidedBy != actor || state.TargetUserID == nil || *state.TargetUserID != target {
			t.Fatalf("state=%+v", state)
		}
		var n int64
		if err := db.Raw("SELECT COUNT(*) FROM tenant_members tm JOIN roles r ON r.id = tm.role_id WHERE tm.tenant_id = ? AND tm.user_id = ? AND r.name = 'Creator' AND r.tenant_id IS NULL AND tm.is_active = true", tenantID, target).Row().Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("creator memberships=%d", n)
		}
		if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 1 {
			t.Fatalf("audit rows=%d", n)
		}
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestDecided) {
			t.Fatalf("replay err=%v want ErrCreatorRequestDecided", err)
		}
		if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 1 {
			t.Fatalf("audit rows after replay=%d", n)
		}
	})

	t.Run("approve new account creates invitation", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		email := uuid.New().String() + "@kel181.test"
		requestID := request(t, f, tenantID, requester, email, nil)
		got, err := decide(actor, requestID, true, "")
		if err != nil || got == nil || got.Status != "approved" || got.InvitationID == nil {
			t.Fatalf("approve: result=%+v err=%v", got, err)
		}
		var invitation struct {
			Email  string
			Token  string
			RoleID uuid.UUID
		}
		if err := db.Raw("SELECT email, token, role_id FROM tenant_invitations WHERE id = ?", *got.InvitationID).Scan(&invitation).Error; err != nil {
			t.Fatal(err)
		}
		if invitation.Email != email || invitation.Token == "" || !isCreatorRole(t, invitation.RoleID) {
			t.Fatalf("invitation=%+v", invitation)
		}
		if n := count(t, "SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ?", tenantID); n != 0 {
			t.Fatalf("members on invitation approve=%d", n)
		}
	})

	t.Run("reject stores trimmed reason and audit then replay is decided", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		email := uuid.New().String() + "@kel181.test"
		requestID := request(t, f, tenantID, requester, email, nil)
		got, err := decide(actor, requestID, false, "  alasan penolakan  ")
		if err != nil || got == nil || got.Status != "rejected" || got.RejectionReason == nil || *got.RejectionReason != "alasan penolakan" {
			t.Fatalf("reject: result=%+v err=%v", got, err)
		}
		var audit struct {
			Decision        string
			RejectionReason *string
		}
		if err := db.Raw("SELECT decision, rejection_reason FROM creator_request_audit WHERE request_id = ?", requestID).Scan(&audit).Error; err != nil {
			t.Fatal(err)
		}
		if audit.Decision != "rejected" || audit.RejectionReason == nil || *audit.RejectionReason != "alasan penolakan" {
			t.Fatalf("audit=%+v", audit)
		}
		if _, err := decide(actor, requestID, false, "lagi"); !errors.Is(err, domain.ErrCreatorRequestDecided) {
			t.Fatalf("replay err=%v want ErrCreatorRequestDecided", err)
		}
	})

	t.Run("reject blank reason invalid keeps pending", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		email := uuid.New().String() + "@kel181.test"
		requestID := request(t, f, tenantID, requester, email, nil)
		if _, err := decide(actor, requestID, false, "   "); !errors.Is(err, domain.ErrCreatorRequestInvalid) {
			t.Fatalf("err=%v want ErrCreatorRequestInvalid", err)
		}
		if state := loadRequest(t, requestID); state.Status != "pending" {
			t.Fatalf("state=%+v", state)
		}
		if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 0 {
			t.Fatalf("audit rows=%d", n)
		}
	})

	t.Run("revoked admin forbidden without side effects", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, false)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		target, email := user(t, f, false)
		requestID := request(t, f, tenantID, requester, email, &target)
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestForbidden) {
			t.Fatalf("err=%v want ErrCreatorRequestForbidden", err)
		}
		if state := loadRequest(t, requestID); state.Status != "pending" {
			t.Fatalf("state=%+v", state)
		}
		if n := count(t, "SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ?", tenantID); n != 0 {
			t.Fatalf("members=%d", n)
		}
		if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 0 {
			t.Fatalf("audit rows=%d", n)
		}
	})

	t.Run("actor without assignment forbidden", func(t *testing.T) {
		f := newFixture(t)
		actor, _ := user(t, f, false)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		email := uuid.New().String() + "@kel181.test"
		requestID := request(t, f, tenantID, requester, email, nil)
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestForbidden) {
			t.Fatalf("err=%v want ErrCreatorRequestForbidden", err)
		}
		if state := loadRequest(t, requestID); state.Status != "pending" {
			t.Fatalf("state=%+v", state)
		}
	})

	t.Run("stale target mismatch", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		emailOwner, email := user(t, f, false)
		otherTarget, _ := user(t, f, false)
		requestID := request(t, f, tenantID, requester, email, nil)
		// Craft a request whose email resolves to a different existing user
		// than its pinned target: the grant must never follow the email.
		if err := db.Exec("UPDATE creator_requests SET target_user_id = ? WHERE id = ?", otherTarget, requestID).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestStale) {
			t.Fatalf("err=%v want ErrCreatorRequestStale", err)
		}
		if n := count(t, "SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ? AND user_id IN (?, ?)", tenantID, emailOwner, otherTarget); n != 0 {
			t.Fatalf("members=%d", n)
		}
		if state := loadRequest(t, requestID); state.Status != "pending" {
			t.Fatalf("state=%+v", state)
		}
	})

	t.Run("parent target stale", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		parent, email := user(t, f, true)
		requestID := request(t, f, tenantID, requester, email, &parent)
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestStale) {
			t.Fatalf("err=%v want ErrCreatorRequestStale", err)
		}
		if n := count(t, "SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ?", tenantID); n != 0 {
			t.Fatalf("members=%d", n)
		}
	})

	// KEL-212: a deleted membership still occupies the unique tenant/user key.
	// Neither deleted nor inactive rows may be recreated or upgraded on approval.
	for _, deleted := range []bool{true, false} {
		name := "inactive membership stale"
		if deleted {
			name = "soft-deleted membership stale"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			actor := admin(t, f, true)
			tenantID := tenant(t, f, "active")
			requester, _ := user(t, f, false)
			target, email := user(t, f, false)
			memberID := uuid.New()
			if err := db.Exec("INSERT INTO tenant_members(id,tenant_id,user_id,role_id,is_active,deleted_at) VALUES (?,?,?,?,?,CASE WHEN ? THEN CURRENT_TIMESTAMP ELSE NULL END)", memberID, tenantID, target, creatorRoleID, deleted, deleted).Error; err != nil {
				t.Fatal(err)
			}
			requestID := request(t, f, tenantID, requester, email, &target)
			if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestStale) {
				t.Fatalf("err=%v want ErrCreatorRequestStale", err)
			}
			if state := loadRequest(t, requestID); state.Status != "pending" || state.DecidedBy != nil || state.InvitationID != nil {
				t.Fatalf("state=%+v", state)
			}
			if n := count(t, "SELECT COUNT(*) FROM tenant_members WHERE tenant_id = ? AND user_id = ?", tenantID, target); n != 1 {
				t.Fatalf("members=%d want original row only", n)
			}
			if n := count(t, "SELECT COUNT(*) FROM tenant_members WHERE id = ? AND role_id = ? AND is_active = ? AND (deleted_at IS NOT NULL) = ?", memberID, creatorRoleID, deleted, deleted); n != 1 {
				t.Fatal("original membership changed")
			}
			if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 0 {
				t.Fatalf("audit rows=%d", n)
			}
			if n := count(t, "SELECT COUNT(*) FROM tenant_invitations WHERE tenant_id = ?", tenantID); n != 0 {
				t.Fatalf("invitations=%d", n)
			}
		})
	}

	t.Run("inactive tenant stale", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "suspended")
		requester, _ := user(t, f, false)
		email := uuid.New().String() + "@kel181.test"
		requestID := request(t, f, tenantID, requester, email, nil)
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorRequestStale) {
			t.Fatalf("err=%v want ErrCreatorRequestStale", err)
		}
		if state := loadRequest(t, requestID); state.Status != "pending" {
			t.Fatalf("state=%+v", state)
		}
	})

	t.Run("target already creator conflict", func(t *testing.T) {
		f := newFixture(t)
		actor := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		target, email := user(t, f, false)
		if err := db.Exec("INSERT INTO tenant_members(id,tenant_id,user_id,role_id,is_active) VALUES (?,?,?,?,true)", uuid.New(), tenantID, target, creatorRoleID).Error; err != nil {
			t.Fatal(err)
		}
		requestID := request(t, f, tenantID, requester, email, &target)
		if _, err := decide(actor, requestID, true, ""); !errors.Is(err, domain.ErrCreatorTargetAlreadyCreator) {
			t.Fatalf("err=%v want ErrCreatorTargetAlreadyCreator", err)
		}
		if state := loadRequest(t, requestID); state.Status != "pending" {
			t.Fatalf("state=%+v", state)
		}
		if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 0 {
			t.Fatalf("audit rows=%d", n)
		}
	})

	t.Run("concurrent approve decides exactly once", func(t *testing.T) {
		f := newFixture(t)
		first := admin(t, f, true)
		second := admin(t, f, true)
		tenantID := tenant(t, f, "active")
		requester, _ := user(t, f, false)
		target, email := user(t, f, false)
		requestID := request(t, f, tenantID, requester, email, &target)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i, actor := range []uuid.UUID{first, second} {
			wg.Add(1)
			go func(i int, actor uuid.UUID) {
				defer wg.Done()
				_, _, _, errs[i] = repo.Decide(context.Background(), requestID, actor, true, "")
			}(i, actor)
		}
		wg.Wait()
		decided := 0
		for _, err := range errs {
			switch {
			case err == nil:
			case errors.Is(err, domain.ErrCreatorRequestDecided):
				decided++
			default:
				t.Fatalf("unexpected err=%v", err)
			}
		}
		if decided != 1 {
			t.Fatalf("errs=%v want exactly one ErrCreatorRequestDecided", errs)
		}
		if n := count(t, "SELECT COUNT(*) FROM creator_request_audit WHERE request_id = ?", requestID); n != 1 {
			t.Fatalf("audit rows=%d", n)
		}
		var n int64
		if err := db.Raw("SELECT COUNT(*) FROM tenant_members tm JOIN roles r ON r.id = tm.role_id WHERE tm.tenant_id = ? AND tm.user_id = ? AND r.name = 'Creator' AND r.tenant_id IS NULL", tenantID, target).Row().Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("members=%d", n)
		}
		if state := loadRequest(t, requestID); state.Status != "approved" {
			t.Fatalf("state=%+v", state)
		}
	})
}
