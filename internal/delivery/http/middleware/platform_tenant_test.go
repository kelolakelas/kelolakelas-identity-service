package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

func TestPlatformTokenCannotReachTenantRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := jwt.NewJWTService("test-secret", time.Hour)
	id := uuid.New()
	platform, _ := tokens.GenerateVerifiedPlatformToken(id, "admin@example.test", nil, 1)
	tenant, _ := tokens.GenerateToken(id, "admin@example.test", uuid.New(), uuid.New(), uuid.New(), false)
	router := gin.New()
	router.Use(AuthMiddleware(tokens), RejectTenantlessPlatform())
	router.GET("/members", func(c *gin.Context) { c.Status(204) })
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"platform only", platform, 403}, {"tenant", tenant, 204}, {"forged", "forged", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/members", nil)
			request.Header.Set("Authorization", "Bearer "+tc.token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status=%d want=%d", response.Code, tc.want)
			}
		})
	}
}
