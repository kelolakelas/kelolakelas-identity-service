package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type settingsTenantRepo struct {
	invitationRoleScopeTenantStub
	conflict  bool
	lookupErr error
	updates   int
	seenName  string
	seenID    uuid.UUID
}

func (r *settingsTenantRepo) IsNameExistsExcept(_ context.Context, name string, id uuid.UUID) (bool, error) {
	r.seenName, r.seenID = name, id
	return r.conflict, r.lookupErr
}
func (r *settingsTenantRepo) Update(_ context.Context, _ *domain.Tenant) error {
	r.updates++
	return nil
}

func TestUpdateTenantSettingsNameAvailability(t *testing.T) {
	id, roleID := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name, requested string
		conflict        bool
		lookupErr       error
		wantErr         error
		updates         int
	}{
		{name: "other tenant name", requested: "Another Tenant", conflict: true, wantErr: domain.ErrTenantNameAlreadyExists},
		{name: "lookup error", requested: "Another Tenant", lookupErr: errors.New("database unavailable")},
		{name: "own name case-only", requested: "CURRENT TENANT", updates: 1},
		{name: "new available name", requested: "New Tenant", updates: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tenant := &domain.Tenant{ID: id, Name: "Current Tenant"}
			repo := &settingsTenantRepo{invitationRoleScopeTenantStub: invitationRoleScopeTenantStub{tenant: tenant}, conflict: tc.conflict, lookupErr: tc.lookupErr}
			u := &tenantUsecase{tenantRepo: repo, permissions: &invitationRoleScopePermissionStub{allowed: true}}
			_, err := u.UpdateTenantSettings(context.Background(), id, roleID, &domain.UpdateTenantSettingsRequest{Name: tc.requested})
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.lookupErr != nil && !errors.Is(err, tc.lookupErr) {
				t.Fatalf("lookup error = %v", err)
			}
			if tc.wantErr == nil && tc.lookupErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.updates != tc.updates {
				t.Fatalf("updates = %d, want %d", repo.updates, tc.updates)
			}
			if repo.seenName != tc.requested || repo.seenID != id {
				t.Fatalf("lookup got %q / %v", repo.seenName, repo.seenID)
			}
			wantName := tc.requested
			if tc.updates == 0 {
				wantName = "Current Tenant"
			}
			if tenant.Name != wantName {
				t.Fatalf("in-memory name changed to %q, want %q", tenant.Name, wantName)
			}
		})
	}
}
