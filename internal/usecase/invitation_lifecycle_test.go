package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type lifecycleInvitationRepo struct {
	invitations []domain.TenantInvitation
}

func (r *lifecycleInvitationRepo) Create(_ context.Context, i *domain.TenantInvitation) error {
	r.invitations = append(r.invitations, *i)
	return nil
}
func (r *lifecycleInvitationRepo) Update(context.Context, *domain.TenantInvitation) error { return nil }
func (r *lifecycleInvitationRepo) GetByTenantAndEmail(context.Context, uuid.UUID, string) (*domain.TenantInvitation, error) {
	return nil, domain.ErrInvitationNotFound
}
func (r *lifecycleInvitationRepo) GetByToken(_ context.Context, token string) (*domain.TenantInvitation, error) {
	for j := range r.invitations {
		if r.invitations[j].Token == token {
			return &r.invitations[j], nil
		}
	}
	return nil, domain.ErrInvitationNotFound
}
func (r *lifecycleInvitationRepo) ReplaceActive(_ context.Context, i *domain.TenantInvitation) error {
	for j := range r.invitations {
		if r.invitations[j].TenantID == i.TenantID && r.invitations[j].Email == i.Email && !r.invitations[j].IsUsed {
			r.invitations[j].IsUsed = true
		}
	}
	r.invitations = append(r.invitations, *i)
	return nil
}
func (r *lifecycleInvitationRepo) ListPending(_ context.Context, tenant uuid.UUID) ([]domain.TenantInvitation, error) {
	var result []domain.TenantInvitation
	for _, i := range r.invitations {
		if i.TenantID == tenant && !i.IsUsed {
			result = append(result, i)
		}
	}
	return result, nil
}
func (r *lifecycleInvitationRepo) Revoke(_ context.Context, tenant, id uuid.UUID) error {
	for j := range r.invitations {
		if r.invitations[j].TenantID == tenant && r.invitations[j].ID == id && !r.invitations[j].IsUsed {
			r.invitations[j].IsUsed = true
			return nil
		}
	}
	return domain.ErrInvitationNotFound
}

type existingInvitationUserRepo struct {
	invitationRoleScopeUserStub
	user *domain.User
	err  error
}

func (r *existingInvitationUserRepo) GetByEmail(context.Context, string) (*domain.User, error) {
	return r.user, r.err
}

func TestInvitationLifecycleResendScopeRevokeAndPermission(t *testing.T) {
	tenant, other := uuid.New(), uuid.New()
	role := uuid.New()
	repo := &lifecycleInvitationRepo{}
	mail := &invitationDeliveryEmailStub{}
	permission := &invitationRoleScopePermissionStub{allowed: true}
	u := NewInvitationUsecase(repo, &invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenant, Name: "Tenant"}}, &invitationRoleScopeUserStub{}, &invitationRoleScopeRbacStub{role: &domain.Role{ID: role, TenantID: &tenant}}, permission, mail)
	ctx := callerContext()
	first, err := u.CreateInvitation(ctx, tenant, role, role, "member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := u.CreateInvitation(ctx, tenant, role, role, "member@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token || mail.calls != 2 {
		t.Fatalf("resend token/calls: %q %q %d", first.Token, second.Token, mail.calls)
	}
	if _, err := u.VerifyInvitation(ctx, first.Token); !errors.Is(err, domain.ErrInvitationUsed) {
		t.Fatalf("old token: %v", err)
	}
	if _, err := u.VerifyInvitation(ctx, second.Token); err != nil {
		t.Fatalf("new token: %v", err)
	}
	repo.invitations = append(repo.invitations, domain.TenantInvitation{ID: uuid.New(), TenantID: other, Token: "other", ExpiresAt: time.Now().Add(time.Hour)})
	list, err := u.ListInvitations(ctx, tenant, role)
	if err != nil || len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := u.RevokeInvitation(ctx, tenant, role, repo.invitations[2].ID); !errors.Is(err, domain.ErrInvitationNotFound) {
		t.Fatalf("foreign revoke: %v", err)
	}
	permission.allowed = false
	if _, err := u.ListInvitations(ctx, tenant, role); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("list permission: %v", err)
	}
	if err := u.RevokeInvitation(ctx, tenant, role, second.ID); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("revoke permission: %v", err)
	}
	permission.allowed = true
	if err := u.RevokeInvitation(ctx, tenant, role, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := u.VerifyInvitation(ctx, second.Token); !errors.Is(err, domain.ErrInvitationUsed) {
		t.Fatalf("revoked verify: %v", err)
	}
}

func TestCreateInvitationRejectsExistingAccountWithoutEmail(t *testing.T) {
	tenant, role := uuid.New(), uuid.New()
	repo := &lifecycleInvitationRepo{}
	mail := &invitationDeliveryEmailStub{}
	u := NewInvitationUsecase(repo, &invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenant}}, &existingInvitationUserRepo{user: &domain.User{ID: uuid.New()}}, &invitationRoleScopeRbacStub{role: &domain.Role{ID: role, TenantID: &tenant}}, &invitationRoleScopePermissionStub{allowed: true}, mail)
	_, err := u.CreateInvitation(callerContext(), tenant, role, role, "member@example.com")
	if !errors.Is(err, domain.ErrUserAlreadyExists) || mail.calls != 0 || len(repo.invitations) != 0 {
		t.Fatalf("existing user: %v emails=%d invitations=%d", err, mail.calls, len(repo.invitations))
	}
}
