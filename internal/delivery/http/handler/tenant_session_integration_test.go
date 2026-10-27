package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
)

func TestKEL224TenantSessionsPostgres(t *testing.T) {
	dsn := os.Getenv("KEL224_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KEL224_TEST_DATABASE_URL to a disposable PostgreSQL database")
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
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	user := domain.User{ID: uuid.New(), Email: uuid.NewString() + "@example.test", FirstName: "Multi", LastName: "Teacher"}
	user.PasswordHash, _ = hash.HashPassword("fixture-password")
	if err = db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM tenant_members WHERE user_id = ?", user.ID); db.Unscoped().Delete(&user) })
	choices := []domain.TenantChoice{}
	for _, name := range []string{"Tenant A", "Tenant B", "Other account tenant"} {
		tenant := domain.Tenant{ID: uuid.New(), Name: name + uuid.NewString(), Status: "active"}
		role := domain.Role{ID: uuid.New(), TenantID: &tenant.ID, Name: name + " role"}
		for _, row := range []any{&tenant, &role} {
			if err = db.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() {
			db.Exec("DELETE FROM tenant_members WHERE tenant_id = ?", tenant.ID)
			db.Exec("DELETE FROM roles WHERE id = ?", role.ID)
			db.Unscoped().Delete(&tenant)
		})
		member := domain.TenantMember{ID: uuid.New(), TenantID: tenant.ID, RoleID: role.ID, UserID: user.ID, IsActive: true}
		if len(choices) < 2 {
			if err = db.Create(&member).Error; err != nil {
				t.Fatal(err)
			}
		}
		choices = append(choices, domain.TenantChoice{TenantID: tenant.ID, TenantName: tenant.Name, RoleID: role.ID, RoleName: role.Name, MemberID: member.ID})
	}
	tokens := jwt.NewJWTService("kel224-test-secret", time.Hour)
	store := repository.NewTenantSessionRepository(db)
	sessions := usecase.NewTenantSessions(store, tokens)
	h := NewTenantSessionHandler(sessions)
	router := gin.New()
	router.GET("/memberships", h.List)
	router.POST("/switch", h.Switch)
	auth := usecase.NewAuthUsecase(repository.NewUserRepository(db), tokens, nil)
	router.POST("/login", NewAuthHandler(auth, nil).Login)
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	login := call("POST", "/login", "", fmt.Sprintf(`{"email":%q,"password":"fixture-password"}`, user.Email))
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if login.Code != 200 || json.Unmarshal(login.Body.Bytes(), &envelope) != nil || envelope.Data.Token == "" {
		t.Fatalf("login %d %s", login.Code, login.Body)
	}
	token := envelope.Data.Token
	w := call("GET", "/memberships", token, "")
	var list struct {
		Data []domain.TenantChoice `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Data) != 2 {
		t.Fatalf("own memberships %d %s", w.Code, w.Body)
	}
	for _, choice := range choices[:2] {
		w = call("POST", "/switch", token, fmt.Sprintf(`{"tenant_id":%q,"role_id":%q,"member_id":%q}`, choice.TenantID, uuid.New(), uuid.New()))
		if w.Code != 200 {
			t.Fatalf("switch %d %s", w.Code, w.Body)
		}
		if json.Unmarshal(w.Body.Bytes(), &envelope) != nil {
			t.Fatal("invalid envelope")
		}
		claims, err := tokens.ValidateToken(envelope.Data.Token)
		if err != nil || claims.UserID != user.ID || claims.TenantID != choice.TenantID || claims.RoleID != choice.RoleID || claims.MemberID != choice.MemberID || claims.IsParent || claims.IsPlatformAdmin {
			t.Fatalf("database claims mismatch: %+v %v", claims, err)
		}
	}
	for _, target := range []uuid.UUID{choices[2].TenantID, uuid.New()} {
		if w = call("POST", "/switch", token, fmt.Sprintf(`{"tenant_id":%q}`, target)); w.Code != 403 {
			t.Fatalf("foreign tenant %d %s", w.Code, w.Body)
		}
	}
	for _, body := range []string{`{}`, `{"tenant_id":"bad"}`, `{"tenant_id":"00000000-0000-0000-0000-000000000000"}`} {
		if w = call("POST", "/switch", token, body); w.Code != 400 {
			t.Fatalf("invalid payload %d", w.Code)
		}
	}
	parent, _ := tokens.GenerateToken(user.ID, user.Email, uuid.Nil, uuid.Nil, uuid.Nil, true)
	platform, _ := tokens.GenerateVerifiedPlatformToken(user.ID, user.Email, nil, 1)
	pending, _ := tokens.GeneratePendingPlatformToken(user.ID, user.Email, 1)
	expiredTokens := jwt.NewJWTService("kel224-test-secret", -time.Hour)
	expired, _ := expiredTokens.GenerateToken(user.ID, user.Email, choices[0].TenantID, choices[0].RoleID, choices[0].MemberID, false)
	for _, tc := range []struct {
		token  string
		status int
	}{{"", 401}, {"bad", 401}, {expired, 401}, {parent, 403}, {platform, 403}, {pending, 403}} {
		for _, route := range []string{"list", "switch"} {
			method, path, body := "GET", "/memberships", ""
			if route == "switch" {
				method, path, body = "POST", "/switch", fmt.Sprintf(`{"tenant_id":%q}`, choices[0].TenantID)
			}
			if w = call(method, path, tc.token, body); w.Code != tc.status {
				t.Fatalf("principal %s: %d want %d", route, w.Code, tc.status)
			}
		}
	}
	// Listing is advisory: recheck deletion, status and role ownership on switch.
	for _, mutation := range []struct {
		sql     string
		restore string
	}{
		{"UPDATE tenant_members SET is_active=false WHERE id=?", "UPDATE tenant_members SET is_active=true WHERE id=?"},
		{"UPDATE tenant_members SET deleted_at=now() WHERE id=?", "UPDATE tenant_members SET deleted_at=NULL WHERE id=?"},
	} {
		if err = db.Exec(mutation.sql, choices[1].MemberID).Error; err != nil {
			t.Fatal(err)
		}
		if w = call("POST", "/switch", token, fmt.Sprintf(`{"tenant_id":%q}`, choices[1].TenantID)); w.Code != 403 {
			t.Fatalf("revoked selection %d %s", w.Code, w.Body)
		}
		if err = db.Exec(mutation.restore, choices[1].MemberID).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A revoke that already holds the row lock must win over an in-flight switch.
	tx := db.Begin()
	if err = tx.Exec("UPDATE tenant_members SET is_active=false WHERE id=?", choices[1].MemberID).Error; err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, _, err := sessions.Switch(ctx, token, choices[1].TenantID); result <- err }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int64
		db.Raw("SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%tenant_members tm%'").Scan(&waiting)
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			tx.Rollback()
			t.Fatal("switch did not wait on concurrent revoke")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, domain.ErrTenantSessionForbidden) {
		t.Fatalf("concurrent revoke: %v", err)
	}
	// A committed reset boundary cannot be bypassed with a previous token.
	boundary := time.Now().UTC().Truncate(time.Second).Add(time.Minute)
	if err = db.Model(&user).Update("session_valid_after", boundary).Error; err != nil {
		t.Fatal(err)
	}
	if w = call("POST", "/switch", token, fmt.Sprintf(`{"tenant_id":%q}`, choices[0].TenantID)); w.Code != 401 {
		t.Fatalf("revoked session %d %s", w.Code, w.Body)
	}
	if w = call("GET", "/memberships", token, ""); w.Code != 401 {
		t.Fatalf("revoked list %d", w.Code)
	}
}
