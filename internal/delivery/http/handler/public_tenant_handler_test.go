package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

type publicTenantStub struct {
	tenant *domain.Tenant
	err    error
	calls  int
}

func (s *publicTenantStub) GetTenantByID(_ context.Context, _ uuid.UUID) (*domain.Tenant, error) {
	s.calls++
	return s.tenant, s.err
}

type publicPolicyStub struct {
	open bool
	err  error
}

func (s publicPolicyStub) Evaluate(context.Context) (domain.PublicCatalogPolicyEvaluated, error) {
	return domain.PublicCatalogPolicyEvaluated{Open: s.open}, s.err
}

func TestPublicTenantProfile(t *testing.T) {
	id := uuid.New()
	secret := "secret"
	address := "Street"
	formatted := "Formatted Street"
	raw := json.RawMessage(`{"private":"secret"}`)
	for _, tc := range []struct {
		name, path, status string
		policy             publicPolicyStub
		err                error
		deleted            bool
		code               int
	}{
		{name: "active public fields", path: id.String(), status: "active", policy: publicPolicyStub{open: true}, code: 200},
		{name: "inactive", path: id.String(), status: "inactive", policy: publicPolicyStub{open: true}, code: 404},
		{name: "deleted", path: id.String(), status: "active", policy: publicPolicyStub{open: true}, deleted: true, code: 404},
		{name: "missing", path: id.String(), policy: publicPolicyStub{open: true}, err: gorm.ErrRecordNotFound, code: 404},
		{name: "invalid UUID", path: "bad", policy: publicPolicyStub{open: true}, code: 404},
		{name: "closed", path: id.String(), policy: publicPolicyStub{}, code: 404},
		{name: "policy failure", path: id.String(), policy: publicPolicyStub{err: errors.New("secret")}, code: 503},
		{name: "store failure", path: id.String(), policy: publicPolicyStub{open: true}, err: errors.New("secret"), code: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &publicTenantStub{tenant: &domain.Tenant{ID: id, Name: "School", Phone: &secret, Address: &address, AddressFormatted: &formatted, About: &raw, PaymentAccountID: &secret, Status: tc.status, DeletedAt: gorm.DeletedAt{Valid: tc.deleted}}, err: tc.err}
			r := gin.New()
			r.GET("/tenants/:id/public", PublicTenantHandler(reader, tc.policy))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/tenants/"+tc.path+"/public", nil))
			if w.Code != tc.code {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			var body struct {
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if tc.code == 200 {
				if len(body.Data) != 3 || body.Data["id"] != id.String() || body.Data["name"] != "School" || body.Data["address"] != formatted {
					t.Fatalf("unexpected public fields: %v", body.Data)
				}
			} else if body.Data != nil {
				t.Fatal("error leaked tenant data")
			}
			if (tc.path == "bad" || !tc.policy.open || tc.policy.err != nil) && reader.calls != 0 {
				t.Fatal("read tenant before gate")
			}
		})
	}
}
