package usecase

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func TestInvitationCreatorGrantDeniedForAllCallers(t *testing.T) {
	tenant := uuid.New()
	for _, tc := range []struct {
		name    string
		allowed bool
		role    *domain.Role
		want    error
	}{
		{"creator caller", true, &domain.Role{Name: "Creator"}, domain.ErrCreatorGrantForbidden},
		{"teacher caller", false, &domain.Role{Name: "Creator"}, domain.ErrPermissionDenied},
		{"custom caller", true, &domain.Role{Name: "Creator"}, domain.ErrCreatorGrantForbidden},
		{"teacher target", true, &domain.Role{Name: "Teacher"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invitations := &invitationRoleScopeInvitationStub{}
			uc := NewInvitationUsecase(invitations, &invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenant, Name: "tenant"}}, &invitationRoleScopeUserStub{}, &invitationRoleScopeRbacStub{role: tc.role}, &invitationRoleScopePermissionStub{allowed: tc.allowed}, &invitationRoleScopeEmailStub{})
			_, err := uc.CreateInvitation(callerContext(), tenant, uuid.New(), uuid.New(), "new@example.com")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if tc.want != nil && invitations.created != nil {
				t.Fatal("forbidden invitation persisted")
			}
		})
	}
}
