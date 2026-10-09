package usecase

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/database"
)

func TestKEL169InvitationPostgresUTC(t *testing.T) {
	dsn := os.Getenv("KEL169_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL169_TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	path, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New((&url.URL{Scheme: "file", Path: path}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	_, _ = m.Close()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewPostgresDB(parsed.Hostname(), parsed.Port(), parsed.User.Username(), "test-only", strings.TrimPrefix(parsed.Path, "/"), "disable", "disable")
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	old := time.Local
	t.Cleanup(func() { time.Local = old })
	for _, zone := range []string{"Asia/Jakarta", "UTC"} {
		t.Run(zone, func(t *testing.T) {
			time.Local, err = time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			tenant := &domain.Tenant{ID: uuid.New(), Name: "kel169-" + uuid.NewString(), Status: "active"}
			if err := db.Create(tenant).Error; err != nil {
				t.Fatal(err)
			}
			if tenant.CreatedAt.Location() != time.UTC {
				t.Fatalf("GORM created_at: %v", tenant.CreatedAt)
			}
			role := &domain.Role{ID: uuid.New(), TenantID: &tenant.ID, Name: "Tutor"}
			if err := db.Create(role).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				db.Exec("DELETE FROM tenant_invitations WHERE tenant_id = ?", tenant.ID)
				db.Exec("DELETE FROM roles WHERE id = ?", role.ID)
				db.Exec("DELETE FROM tenants WHERE id = ?", tenant.ID)
			})
			repo := repository.NewInvitationRepository(db)
			u := NewInvitationUsecase(repo, &invitationRoleScopeTenantStub{tenant: tenant}, &invitationRoleScopeUserStub{}, &invitationRoleScopeRbacStub{role: role}, &invitationRoleScopePermissionStub{allowed: true}, &invitationDeliveryEmailStub{})
			inv, err := u.CreateInvitation(callerContext(), tenant.ID, role.ID, role.ID, "member@example.com")
			if err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetByToken(callerContext(), inv.Token)
			if err != nil {
				t.Fatal(err)
			}
			if stored.ExpiresAt.Sub(stored.CreatedAt) != 48*time.Hour || stored.ExpiresAt.Location() != time.UTC || !stored.ExpiresAt.Equal(inv.ExpiresAt.Truncate(time.Microsecond)) {
				t.Fatalf("round-trip shifted: %+v", stored)
			}
			data, err := json.Marshal(stored)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "+07:00") {
				t.Fatalf("local JSON: %s", data)
			}
			if _, err := u.VerifyInvitation(callerContext(), inv.Token); err != nil {
				t.Fatal(err)
			}
			if _, err := repository.NewUserRepository(db).RegisterInvitedUserTx(callerContext(), inv.Token, "First", "Last", "test-only-password"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				db.Exec("DELETE FROM user_wallets WHERE user_id IN (SELECT id FROM users WHERE email = ?)", inv.Email)
				db.Exec("DELETE FROM tenant_members WHERE tenant_id = ?", tenant.ID)
				db.Exec("DELETE FROM users WHERE email = ?", inv.Email)
			})
			if _, err := u.VerifyInvitation(callerContext(), inv.Token); !errors.Is(err, domain.ErrInvitationUsed) {
				t.Fatalf("used: %v", err)
			}
		})
	}
}
