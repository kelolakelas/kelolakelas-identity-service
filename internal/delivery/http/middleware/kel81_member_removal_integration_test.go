package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/handler"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/middleware"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

// TestKEL81MemberRemovalOverHTTP runs DELETE /api/v1/members/:id end to end against the
// migrated PostgreSQL database named by KEL76_TEST_DATABASE_URL (skips without it).
func TestKEL81MemberRemovalOverHTTP(t *testing.T) {
	db := openKEL76Database(t)
	f := seedKEL76(t, db)

	jwtService := jwt.NewJWTService("kel81-integration-secret-0123456789abcdef", time.Hour)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	memberHandler := handler.NewMemberHandler(usecase.NewMemberUsecase(repository.NewMemberRepository(db)))
	router.DELETE("/api/v1/members/:id", middleware.AuthMiddleware(jwtService), memberHandler.DeleteMember)

	remove := func(t *testing.T, caller kel76Member, callerMemberID uuid.UUID, roleID, target uuid.UUID) int {
		t.Helper()
		token, err := jwtService.GenerateToken(caller.userID, "kel81@example.com", f.tenantA, roleID, callerMemberID, false)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/members/"+target.String(), nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	isRemoved := func(t *testing.T, memberID uuid.UUID) bool {
		t.Helper()
		var removed bool
		if err := db.Raw("SELECT deleted_at IS NOT NULL FROM tenant_members WHERE id = ?", memberID).Row().Scan(&removed); err != nil {
			t.Fatal(err)
		}
		return removed
	}

	t.Run("admin removes another member, whose old token is then denied", func(t *testing.T) {
		admin := addMember(t, db, f.tenantA, f.adminRole)
		target := addMember(t, db, f.tenantA, f.editorRole)
		if got := remove(t, admin, admin.memberID, f.adminRole, target.memberID); got != http.StatusOK {
			t.Fatalf("status = %d, want 200", got)
		}
		if !isRemoved(t, target.memberID) {
			t.Fatal("target membership was not soft-deleted")
		}
		// The removed member's token still names an editor role with member:delete; the
		// active-membership check (KEL-76) must refuse it.
		if got := remove(t, target, target.memberID, f.editorRole, admin.memberID); got != http.StatusForbidden {
			t.Fatalf("removed member's old token: status = %d, want 403", got)
		}
		if isRemoved(t, admin.memberID) {
			t.Fatal("removed member's old token deleted the admin")
		}
		if got := remove(t, admin, admin.memberID, f.adminRole, target.memberID); got != http.StatusNotFound {
			t.Fatalf("removing again: status = %d, want 404", got)
		}
	})
	t.Run("own membership is refused with 409 and kept", func(t *testing.T) {
		admin := addMember(t, db, f.tenantA, f.adminRole)
		if got := remove(t, admin, admin.memberID, f.adminRole, admin.memberID); got != http.StatusConflict {
			t.Fatalf("member_id token: status = %d, want 409", got)
		}
		if got := remove(t, admin, uuid.Nil, f.adminRole, admin.memberID); got != http.StatusConflict {
			t.Fatalf("legacy token: status = %d, want 409", got)
		}
		if isRemoved(t, admin.memberID) {
			t.Fatal("own membership was deleted")
		}
	})
	t.Run("member without member:delete is denied and nothing changes", func(t *testing.T) {
		viewer := addMember(t, db, f.tenantA, f.viewerRole)
		target := addMember(t, db, f.tenantA, f.editorRole)
		if got := remove(t, viewer, viewer.memberID, f.viewerRole, target.memberID); got != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", got)
		}
		if isRemoved(t, target.memberID) {
			t.Fatal("target removed without permission")
		}
	})
	t.Run("member of another tenant is not found and kept", func(t *testing.T) {
		admin := addMember(t, db, f.tenantA, f.adminRole)
		foreign := addMember(t, db, f.tenantB, f.foreignRole)
		if got := remove(t, admin, admin.memberID, f.adminRole, foreign.memberID); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
		if isRemoved(t, foreign.memberID) {
			t.Fatal("foreign tenant member was removed")
		}
	})
}
