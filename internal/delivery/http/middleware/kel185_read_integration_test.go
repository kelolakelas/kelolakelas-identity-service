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

func TestKEL185HTTPReadMatrix(t *testing.T) {
	db := openKEL76Database(t)
	f := seedKEL76(t, db)
	for _, permission := range []string{"member:read", "role:read", "tenant:read"} {
		mustExec(t, db, "INSERT INTO role_permissions(role_id,permission_id) SELECT ?,id FROM permissions WHERE name=?", f.adminRole, permission)
	}
	members := repository.NewMemberRepository(db)
	tokens := jwt.NewJWTService("kel185-read-integration", time.Hour)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.AuthMiddleware(tokens))
	mh := handler.NewMemberHandler(usecase.NewMemberUsecase(members))
	rh := handler.NewRoleHandler(usecase.NewRoleUsecase(repository.NewRbacRepository(db), members))
	th := handler.NewTenantHandler(usecase.NewTenantUsecase(nil, repository.NewTenantRepository(db), members, nil, nil, nil))
	r.GET("/members", mh.ListMembers)
	r.GET("/members/me/membership", mh.GetMyMembership)
	r.GET("/members/:id", mh.GetMember)
	r.GET("/roles", rh.GetRoles)
	r.GET("/settings", th.GetSettings)
	r.GET("/location", th.GetLocation)
	for _, tc := range []struct {
		name   string
		role   uuid.UUID
		active bool
		want   int
	}{
		{"all reads", f.adminRole, true, 200}, {"no read permission", f.viewerRole, true, 403}, {"inactive", f.adminRole, false, 403}, {"foreign role", f.foreignRole, true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := addMember(t, db, f.tenantA, tc.role)
			if !tc.active {
				mustExec(t, db, "UPDATE tenant_members SET is_active=false WHERE id=?", m.memberID)
			}
			token, err := tokens.GenerateToken(m.userID, "read@example.com", f.tenantA, tc.role, m.memberID, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/members", "/members/" + m.memberID.String(), "/roles", "/settings", "/location"} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)
				if rec.Code != tc.want {
					t.Fatalf("%s status %d want %d: %s", path, rec.Code, tc.want, rec.Body.String())
				}
			}
			if tc.name == "no read permission" {
				req := httptest.NewRequest(http.MethodGet, "/members/me/membership", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)
				if rec.Code != 200 {
					t.Fatalf("bootstrap %d", rec.Code)
				}
			}
		})
	}
}
