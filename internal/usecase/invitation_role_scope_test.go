package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// invitationRoleScopePermissionStub grants or denies the caller permission.
type invitationRoleScopePermissionStub struct {
	allowed bool
	err     error
}

func (s *invitationRoleScopePermissionStub) HasActiveMemberPermission(context.Context, domain.MemberPermissionQuery) (bool, error) {
	return s.allowed, s.err
}

type invitationRoleScopeTenantStub struct{ tenant *domain.Tenant }

func (s *invitationRoleScopeTenantStub) Create(context.Context, *domain.Tenant) error { return nil }
func (s *invitationRoleScopeTenantStub) GetByID(_ context.Context, id uuid.UUID) (*domain.Tenant, error) {
	if s.tenant == nil {
		return nil, errors.New("tenant not found")
	}
	return s.tenant, nil
}
func (s *invitationRoleScopeTenantStub) IsNameExistsExcept(context.Context, string, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *invitationRoleScopeTenantStub) IsNameExists(context.Context, string) (bool, error) {
	return false, nil
}
func (s *invitationRoleScopeTenantStub) Update(context.Context, *domain.Tenant) error { return nil }
func (s *invitationRoleScopeTenantStub) Delete(context.Context, uuid.UUID) error      { return nil }

type invitationRoleScopeUserStub struct{}

func (s *invitationRoleScopeUserStub) Create(context.Context, *domain.User) error { return nil }
func (s *invitationRoleScopeUserStub) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	return nil, errors.New("not found")
}
func (s *invitationRoleScopeUserStub) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (s *invitationRoleScopeUserStub) Update(context.Context, *domain.User) error { return nil }
func (s *invitationRoleScopeUserStub) Delete(context.Context, uuid.UUID) error    { return nil }
func (s *invitationRoleScopeUserStub) RegisterTenantTx(context.Context, *domain.User, *domain.Tenant) (*domain.TenantMember, error) {
	return nil, errors.New("not implemented")
}
func (s *invitationRoleScopeUserStub) RegisterInvitedUserTx(context.Context, string, string, string, string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
func (s *invitationRoleScopeUserStub) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s *invitationRoleScopeUserStub) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	return nil, errors.New("not found")
}

type invitationRoleScopeInvitationStub struct{ created *domain.TenantInvitation }

func (s *invitationRoleScopeInvitationStub) Create(_ context.Context, invitation *domain.TenantInvitation) error {
	clone := *invitation
	s.created = &clone
	return nil
}
func (s *invitationRoleScopeInvitationStub) GetByToken(context.Context, string) (*domain.TenantInvitation, error) {
	return nil, domain.ErrInvitationNotFound
}
func (s *invitationRoleScopeInvitationStub) GetByTenantAndEmail(context.Context, uuid.UUID, string) (*domain.TenantInvitation, error) {
	return nil, domain.ErrInvitationNotFound
}
func (s *invitationRoleScopeInvitationStub) Update(context.Context, *domain.TenantInvitation) error {
	return nil
}
func (s *invitationRoleScopeInvitationStub) ReplaceActive(ctx context.Context, invitation *domain.TenantInvitation) error {
	return s.Create(ctx, invitation)
}
func (s *invitationRoleScopeInvitationStub) ListPending(context.Context, uuid.UUID) ([]domain.TenantInvitation, error) {
	return nil, nil
}
func (s *invitationRoleScopeInvitationStub) Revoke(context.Context, uuid.UUID, uuid.UUID) error {
	return domain.ErrInvitationNotFound
}

type invitationRoleScopeRbacStub struct {
	role *domain.Role
	err  error
}

func (s *invitationRoleScopeRbacStub) GetPermissions(context.Context) ([]domain.Permission, error) {
	return nil, nil
}
func (s *invitationRoleScopeRbacStub) GetRolesByTenantID(context.Context, uuid.UUID) ([]domain.Role, error) {
	return nil, nil
}
func (s *invitationRoleScopeRbacStub) GetRoleByID(context.Context, uuid.UUID) (*domain.Role, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.role, nil
}
func (s *invitationRoleScopeRbacStub) GetRoleByNameAndTenantID(context.Context, string, uuid.UUID) (*domain.Role, error) {
	return nil, nil
}
func (s *invitationRoleScopeRbacStub) CreateRoleTx(context.Context, *domain.Role, []uuid.UUID) error {
	return nil
}
func (s *invitationRoleScopeRbacStub) UpdateRoleTx(context.Context, uuid.UUID, string, string, []uuid.UUID) error {
	return nil
}
func (s *invitationRoleScopeRbacStub) DeleteRole(context.Context, uuid.UUID) error { return nil }
func (s *invitationRoleScopeRbacStub) CountMembersByRole(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

type invitationRoleScopeEmailStub struct{ sent bool }

func (s *invitationRoleScopeEmailStub) SendPasswordResetEmail(string, string) error { return nil }

func (s *invitationRoleScopeEmailStub) SendInvitationEmail(string, string, string) error {
	s.sent = true
	return nil
}

// TestCreateInvitationRejectsRoleFromAnotherTenant is acceptance criterion 1: an invitation
// carrying a role owned by a different tenant must be rejected and no invitation created.
func TestCreateInvitationRejectsRoleFromAnotherTenant(t *testing.T) {
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	callerRoleID := uuid.New()
	foreignRoleID := uuid.New()

	invitationRepo := &invitationRoleScopeInvitationStub{}
	emailService := &invitationRoleScopeEmailStub{}
	usecase := NewInvitationUsecase(
		invitationRepo,
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{role: &domain.Role{ID: foreignRoleID, TenantID: &otherTenantID, Name: "Foreign"}},
		&invitationRoleScopePermissionStub{allowed: true},
		emailService,
	)

	_, err := usecase.CreateInvitation(callerContext(), tenantID, callerRoleID, foreignRoleID, "member@example.com")
	if !errors.Is(err, domain.ErrInvitationRoleInvalid) {
		t.Fatalf("error = %v, want ErrInvitationRoleInvalid", err)
	}
	if invitationRepo.created != nil {
		t.Fatalf("invitation was created: %+v", invitationRepo.created)
	}
	if emailService.sent {
		t.Fatal("invitation email was sent for a rejected role")
	}
}

// TestCreateInvitationAcceptsTenantOwnedRole proves the tenant's own role still works.
func TestCreateInvitationAcceptsTenantOwnedRole(t *testing.T) {
	tenantID := uuid.New()
	callerRoleID := uuid.New()
	ownedRoleID := uuid.New()

	invitationRepo := &invitationRoleScopeInvitationStub{}
	usecase := NewInvitationUsecase(
		invitationRepo,
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{role: &domain.Role{ID: ownedRoleID, TenantID: &tenantID, Name: "Staff"}},
		&invitationRoleScopePermissionStub{allowed: true},
		&invitationRoleScopeEmailStub{},
	)

	invitation, err := usecase.CreateInvitation(callerContext(), tenantID, callerRoleID, ownedRoleID, "member@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if invitation == nil || invitation.RoleID != ownedRoleID || invitation.TenantID != tenantID {
		t.Fatalf("invitation = %+v", invitation)
	}
	if invitationRepo.created == nil {
		t.Fatal("invitation was not created")
	}
}

// TestCreateInvitationAcceptsSystemRoleForAnyTenant covers the edge case of a system role
// (tenant_id NULL) being usable inside any tenant.
func TestCreateInvitationAcceptsSystemRoleForAnyTenant(t *testing.T) {
	tenantID := uuid.New()
	systemRoleID := uuid.New()

	invitationRepo := &invitationRoleScopeInvitationStub{}
	usecase := NewInvitationUsecase(
		invitationRepo,
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{role: &domain.Role{ID: systemRoleID, TenantID: nil, Name: "Teacher"}},
		&invitationRoleScopePermissionStub{allowed: true},
		&invitationRoleScopeEmailStub{},
	)

	invitation, err := usecase.CreateInvitation(callerContext(), tenantID, uuid.New(), systemRoleID, "member@example.com")
	if err != nil {
		t.Fatalf("system role rejected: %v", err)
	}
	if invitation == nil || invitation.RoleID != systemRoleID {
		t.Fatalf("invitation = %+v", invitation)
	}
}

// TestCreateInvitationRejectsUnknownRole proves a deleted or unknown role is reported as a
// validation error instead of leaking a not-found signal, and never creates an invitation.
func TestCreateInvitationRejectsUnknownRole(t *testing.T) {
	tenantID := uuid.New()

	invitationRepo := &invitationRoleScopeInvitationStub{}
	usecase := NewInvitationUsecase(
		invitationRepo,
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{err: domain.ErrRoleNotFound},
		&invitationRoleScopePermissionStub{allowed: true},
		&invitationRoleScopeEmailStub{},
	)

	_, err := usecase.CreateInvitation(callerContext(), tenantID, uuid.New(), uuid.New(), "member@example.com")
	if !errors.Is(err, domain.ErrInvitationRoleInvalid) {
		t.Fatalf("error = %v, want ErrInvitationRoleInvalid", err)
	}
	if invitationRepo.created != nil {
		t.Fatalf("invitation was created: %+v", invitationRepo.created)
	}
}

// TestCreateInvitationStillRequiresPermission ensures the role guard runs after, and never
// instead of, the permission check.
func TestCreateInvitationStillRequiresPermission(t *testing.T) {
	tenantID := uuid.New()

	usecase := NewInvitationUsecase(
		&invitationRoleScopeInvitationStub{},
		&invitationRoleScopeTenantStub{tenant: &domain.Tenant{ID: tenantID, Name: "Tenant A"}},
		&invitationRoleScopeUserStub{},
		&invitationRoleScopeRbacStub{role: &domain.Role{ID: uuid.New(), TenantID: &tenantID, Name: "Staff"}},
		&invitationRoleScopePermissionStub{allowed: false},
		&invitationRoleScopeEmailStub{},
	)

	_, err := usecase.CreateInvitation(callerContext(), tenantID, uuid.New(), uuid.New(), "member@example.com")
	if !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatalf("error = %v, want permission denied", err)
	}
}
