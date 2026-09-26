package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type platformQueueStub struct {
	items []domain.PlatformCreatorRequest
	err   error
}

func (s platformQueueStub) ListPlatform(_ context.Context) ([]domain.PlatformCreatorRequest, error) {
	return s.items, s.err
}

func TestPlatformCreatorRequestHandlerProjectionAndFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		stub platformQueueStub
		want int
	}{
		{"success", platformQueueStub{items: []domain.PlatformCreatorRequest{{ID: uuid.New(), TenantName: "Tenant A", Status: "pending"}}}, 200},
		{"failure", platformQueueStub{err: errors.New("db unavailable")}, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/api/v1/platform/creator-requests", NewPlatformCreatorRequestHandler(tc.stub).List)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/platform/creator-requests", nil))
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.want == 200 {
				items := body["data"].([]any)
				if len(items) != 1 || items[0].(map[string]any)["tenant_name"] != "Tenant A" {
					t.Fatalf("body=%v", body)
				}
			} else if body["data"] != nil {
				t.Fatalf("failure disclosed data: %v", body)
			}
		})
	}
}
