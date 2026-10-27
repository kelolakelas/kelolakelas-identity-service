package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestKEL225ExistingInvitationPostgres(t *testing.T) {
	dsn := os.Getenv("KEL225_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL225_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	dir, _ := filepath.Abs("../../../../migrations")
	m, err := migrate.New((&url.URL{Scheme: "file", Path: dir}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	must := func(row any) {
		t.Helper()
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", FirstName: "Existing", LastName: "Teacher"}
	user.PasswordHash, _ = hash.HashPassword("Fixture225!")
	must(&user)
	wallet := domain.UserWallet{ID: uuid.New(), UserID: user.ID, Balance: 125}
	must(&wallet)
	teacher := domain.Role{ID: uuid.New(), Name: "Teacher"}
	must(&teacher)
	source := domain.Tenant{ID: uuid.New(), Name: uuid.NewString(), Status: "active"}
	must(&source)
	old := domain.TenantMember{ID: uuid.New(), TenantID: source.ID, UserID: user.ID, RoleID: teacher.ID, IsActive: true}
	must(&old)
	tokens := jwt.NewJWTService("kel225-test-secret", time.Hour)
	token, _ := tokens.GenerateToken(user.ID, user.Email, source.ID, teacher.ID, old.ID, false)
	repo := repository.NewInvitationRepository(db)
	service := usecase.NewExistingInvitations(repo, tokens)
	router := gin.New()
	router.POST("/accept", AcceptExistingInvitation(service))
	call := func(access, invite string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/accept", strings.NewReader(fmt.Sprintf(`{"token":%q,"tenant_id":%q,"role_id":%q,"user_id":%q}`, invite, uuid.New(), uuid.New(), uuid.New())))
		req.Header.Set("Content-Type", "application/json")
		if access != "" {
			req.Header.Set("Authorization", "Bearer "+access)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	invitation := func() domain.TenantInvitation {
		target := domain.Tenant{ID: uuid.New(), Name: uuid.NewString(), Status: "active"}
		must(&target)
		i := domain.TenantInvitation{ID: uuid.New(), TenantID: target.ID, RoleID: teacher.ID, Email: strings.ToUpper(user.Email), Token: uuid.NewString(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
		must(&i)
		return i
	}
	ctx := context.Background()
	initial := invitation()
	if err := repo.ValidateExistingTeacher(ctx, initial.TenantID, teacher.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if w := call("", initial.Token); w.Code != 401 {
		t.Fatalf("anonymous %d", w.Code)
	}
	wrong, _ := tokens.GenerateToken(uuid.New(), "other@example.test", source.ID, teacher.ID, old.ID, false)
	if w := call(wrong, initial.Token); w.Code != 401 {
		t.Fatalf("missing user %d", w.Code)
	}
	other := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", FirstName: "Wrong", LastName: "Account", PasswordHash: "hash"}
	must(&other)
	wrong, _ = tokens.GenerateToken(other.ID, other.Email, source.ID, teacher.ID, old.ID, false)
	if w := call(wrong, initial.Token); w.Code != 403 {
		t.Fatalf("wrong account %d", w.Code)
	}
	parentToken, _ := tokens.GenerateToken(user.ID, user.Email, uuid.Nil, uuid.Nil, uuid.Nil, true)
	if w := call(parentToken, initial.Token); w.Code != 403 {
		t.Fatalf("parent %d", w.Code)
	}
	// First-use concurrency, not just concurrent replays.
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- call(token, initial.Token).Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent accept %d", code)
		}
	}
	var accepted domain.TenantMember
	if err := db.Where("tenant_id = ? AND user_id = ?", initial.TenantID, user.ID).First(&accepted).Error; err != nil {
		t.Fatal(err)
	}
	if accepted.RoleID != teacher.ID {
		t.Fatal("forged role trusted")
	}
	if err := repo.ValidateExistingTeacher(ctx, initial.TenantID, teacher.ID, user.ID); !errors.Is(err, domain.ErrAlreadyTenantMember) {
		t.Fatalf("same tenant %v", err)
	}
	// Session selection uses the unchanged KEL-224 path, with live DB-derived claims.
	sessions := usecase.NewTenantSessions(repository.NewTenantSessionRepository(db), tokens)
	for _, target := range []uuid.UUID{initial.TenantID, source.ID} {
		access, choice, err := sessions.Switch(ctx, token, target)
		if err != nil {
			t.Fatal(err)
		}
		c, err := tokens.ValidateToken(access)
		if err != nil || c.UserID != user.ID || c.TenantID != target || c.MemberID != choice.MemberID {
			t.Fatal("switch claims")
		}
	}
	var after domain.User
	db.First(&after, "id = ?", user.ID)
	if after.PasswordHash != user.PasswordHash {
		t.Fatal("password changed")
	}
	var oldAfter domain.TenantMember
	db.First(&oldAfter, "id = ?", old.ID)
	if oldAfter.RoleID != old.RoleID || !oldAfter.IsActive {
		t.Fatal("old membership changed")
	}
	for table, want := range map[string]int64{"users": 2, "user_wallets": 1, "tenant_members": 2} {
		var count int64
		query := db.Table(table)
		if table == "users" {
			query = query.Where("id IN ?", []uuid.UUID{user.ID, other.ID})
		} else {
			query = query.Where("user_id = ?", user.ID)
		}
		query.Count(&count)
		if count != want {
			t.Fatalf("%s count %d", table, count)
		}
	}
	for _, kind := range []string{"expired", "revoked", "replaced", "inactive", "deleted", "foreign-role", "creator", "no-teacher", "reset", "existing-target", "two-invitations", "role-preserved", "rollback"} {
		t.Run(kind, func(t *testing.T) {
			i := invitation()
			expected := 200
			switch kind {
			case "expired":
				db.Model(&i).Update("expires_at", time.Now().UTC().Add(-time.Hour))
				expected = 400
			case "revoked":
				if err := repo.Revoke(ctx, i.TenantID, i.ID); err != nil {
					t.Fatal(err)
				}
				expected = 409
			case "replaced":
				next := i
				next.ID = uuid.New()
				next.Token = uuid.NewString()
				if err := repo.ReplaceActive(ctx, &next); err != nil {
					t.Fatal(err)
				}
				expected = 409
			case "inactive", "deleted", "existing-target":
				member := domain.TenantMember{ID: uuid.New(), TenantID: i.TenantID, UserID: user.ID, RoleID: teacher.ID, IsActive: true}
				must(&member)
				if kind == "inactive" {
					db.Model(&member).Update("is_active", false)
					expected = 409
				}
				if kind == "deleted" {
					db.Delete(&member)
					expected = 409
				}
			case "foreign-role", "creator":
				role := domain.Role{ID: uuid.New(), Name: "Creator"}
				if kind == "foreign-role" {
					role.Name = "Teacher"
					role.TenantID = &source.ID
				}
				must(&role)
				db.Model(&i).Update("role_id", role.ID)
				expected = 403
			case "no-teacher":
				db.Model(&old).Update("is_active", false)
				db.Model(&accepted).Update("is_active", false)
				expected = 403
				defer db.Model(&old).Update("is_active", true)
				defer db.Model(&accepted).Update("is_active", true)
			case "reset":
				now := time.Now().UTC().Add(time.Minute)
				db.Model(&user).Update("session_valid_after", now)
				expected = 401
				defer db.Model(&user).Update("session_valid_after", nil)
			case "two-invitations":
				second := i
				second.ID = uuid.New()
				second.Token = uuid.NewString()
				must(&second)
				var concurrent sync.WaitGroup
				outcomes := make(chan int, 2)
				for _, candidate := range []string{i.Token, second.Token} {
					concurrent.Add(1)
					go func(value string) { defer concurrent.Done(); outcomes <- call(token, value).Code }(candidate)
				}
				concurrent.Wait()
				close(outcomes)
				for code := range outcomes {
					if code != 200 {
						t.Fatalf("two invitations: %d", code)
					}
				}
				var count int64
				db.Model(&domain.TenantMember{}).Where("tenant_id = ? AND user_id = ?", i.TenantID, user.ID).Count(&count)
				if count != 1 {
					t.Fatal("duplicate target memberships")
				}
			case "role-preserved":
				role := domain.Role{ID: uuid.New(), TenantID: &i.TenantID, Name: "Existing role"}
				must(&role)
				member := domain.TenantMember{ID: uuid.New(), TenantID: i.TenantID, UserID: user.ID, RoleID: role.ID, IsActive: true}
				must(&member)
				if w := call(token, i.Token); w.Code != 200 {
					t.Fatalf("existing target: %d", w.Code)
				}
				var saved domain.TenantMember
				db.First(&saved, "id = ?", member.ID)
				if saved.RoleID != role.ID {
					t.Fatal("existing target role overwritten")
				}
			case "rollback":
				db.Exec(`CREATE FUNCTION kel225_refuse_consume() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture rollback'; END $$`)
				db.Exec(`CREATE TRIGGER kel225_refuse_consume BEFORE UPDATE ON tenant_invitations FOR EACH ROW EXECUTE FUNCTION kel225_refuse_consume()`)
				expected = 500
				defer db.Exec(`DROP FUNCTION kel225_refuse_consume() CASCADE`)
			}
			w := call(token, i.Token)
			if w.Code != expected {
				t.Fatalf("%s %d %s", kind, w.Code, w.Body)
			}
			if kind == "rollback" {
				var count int64
				db.Model(&domain.TenantMember{}).Where("tenant_id = ?", i.TenantID).Count(&count)
				if count != 0 {
					t.Fatal("membership survived rollback")
				}
			}
			if expected == 200 {
				var body map[string]any
				if json.Unmarshal(w.Body.Bytes(), &body) != nil {
					t.Fatal("invalid response")
				}
			}
		})
	}
	// Removing an accepted membership never lets replay restore it.
	db.Model(&accepted).Update("is_active", false)
	if w := call(token, initial.Token); w.Code != 409 {
		t.Fatalf("replay reactivated %d", w.Code)
	}
}
