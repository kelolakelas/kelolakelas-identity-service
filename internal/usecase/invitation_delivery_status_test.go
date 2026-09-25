package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// invitationDeliveryEmailStub records delivery attempts and can be made to fail.
type invitationDeliveryEmailStub struct {
	err   error
	calls int
}

func (s *invitationDeliveryEmailStub) SendPasswordResetEmail(string, string) error { return nil }

func (s *invitationDeliveryEmailStub) SendInvitationEmail(string, string, string) error {
	s.calls++
	return s.err
}

// TestCreateInvitationMarksEmailSentOnDeliverySuccess is acceptance criterion 2:
// when Resend accepts the invitation email, the response reports it as sent.
func TestCreateInvitationMarksEmailSentOnDeliverySuccess(t *testing.T) {
	tenantID := uuid.New()
	invitationRepo := &invitationRoleScopeInvitationStub{}
	emailService := &invitationDeliveryEmailStub{}

	usecase := NewInvitationUsecase(
		invitationRepo,
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{role: &domain.Role{ID: uuid.New(), TenantID: &tenantID, Name: "Staff"}},
		&invitationRoleScopePermissionStub{allowed: true},
		emailService,
	)

	invitation, err := usecase.CreateInvitation(context.Background(), tenantID, uuid.New(), uuid.New(), "member@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invitation == nil {
		t.Fatal("invitation is nil")
	}
	if !invitation.EmailSent {
		t.Fatal("invitation.EmailSent = false, want true after a successful delivery")
	}
	if emailService.calls != 1 {
		t.Fatalf("delivery attempts = %d, want 1", emailService.calls)
	}
}

// TestCreateInvitationKeepsInvitationWhenDeliveryFails is acceptance criterion 1:
// a Resend failure must not fail the stored invitation; the response carries the
// invitation with EmailSent=false instead of an error.
func TestCreateInvitationKeepsInvitationWhenDeliveryFails(t *testing.T) {
	tenantID := uuid.New()
	invitationRepo := &invitationRoleScopeInvitationStub{}
	emailService := &invitationDeliveryEmailStub{err: errors.New("resend unavailable")}

	usecase := NewInvitationUsecase(
		invitationRepo,
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{role: &domain.Role{ID: uuid.New(), TenantID: &tenantID, Name: "Staff"}},
		&invitationRoleScopePermissionStub{allowed: true},
		emailService,
	)

	invitation, err := usecase.CreateInvitation(context.Background(), tenantID, uuid.New(), uuid.New(), "member@example.com")
	if err != nil {
		t.Fatalf("invitation creation failed on delivery error: %v", err)
	}
	if invitation == nil {
		t.Fatal("invitation is nil")
	}
	if invitation.EmailSent {
		t.Fatal("invitation.EmailSent = true, want false after a failed delivery")
	}
	if invitationRepo.created == nil {
		t.Fatal("invitation was not stored despite the delivery failure")
	}
	if emailService.calls != 1 {
		t.Fatalf("delivery attempts = %d, want 1", emailService.calls)
	}
}
