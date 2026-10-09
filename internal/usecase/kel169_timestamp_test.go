package usecase

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func TestKEL169InvitationUTC(t *testing.T) {
	old := time.Local
	t.Cleanup(func() { time.Local = old })
	for _, zone := range []string{"Asia/Jakarta", "UTC"} {
		t.Run(zone, func(t *testing.T) {
			var err error
			time.Local, err = time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			tenant, role := uuid.New(), uuid.New()
			repo := &lifecycleInvitationRepo{}
			u := NewInvitationUsecase(repo, &invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenant, Name: "Tenant"}}, &invitationRoleScopeUserStub{}, &invitationRoleScopeRbacStub{role: &domain.Role{ID: role, TenantID: &tenant}}, &invitationRoleScopePermissionStub{allowed: true}, &invitationDeliveryEmailStub{})
			inv, err := u.CreateInvitation(callerContext(), tenant, role, role, "member@example.com")
			if err != nil {
				t.Fatal(err)
			}
			for _, stamp := range []time.Time{inv.CreatedAt, inv.UpdatedAt, inv.ExpiresAt, repo.invitations[0].ExpiresAt} {
				if stamp.Location() != time.UTC {
					t.Fatalf("timestamp not UTC: %v", stamp)
				}
			}
			if inv.ExpiresAt.Sub(inv.CreatedAt) != 48*time.Hour {
				t.Fatal("invitation TTL is not exactly 48h")
			}
			data, err := json.Marshal(inv)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "+07:00") || !strings.Contains(string(data), "Z\"") {
				t.Fatalf("non-UTC JSON: %s", data)
			}
			if _, err := u.VerifyInvitation(callerContext(), inv.Token); err != nil {
				t.Fatal(err)
			}
			repo.invitations[0].ExpiresAt = time.Now().UTC().Add(-time.Second)
			if _, err := u.VerifyInvitation(callerContext(), inv.Token); !errors.Is(err, domain.ErrInvitationExpired) {
				t.Fatalf("expired: %v", err)
			}
			if _, err := u.VerifyInvitation(callerContext(), "missing"); !errors.Is(err, domain.ErrInvitationNotFound) {
				t.Fatalf("missing: %v", err)
			}
		})
	}
}
