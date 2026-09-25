package usecase

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/email"
)

type invitationUsecase struct {
	invitationRepo repository.InvitationRepository
	tenantRepo     repository.TenantRepository
	userRepo       domain.UserRepository
	rbacRepo       repository.RbacRepository
	permissions    PermissionChecker
	emailService   email.EmailService
}

func NewInvitationUsecase(
	invitationRepo repository.InvitationRepository,
	tenantRepo repository.TenantRepository,
	userRepo domain.UserRepository,
	rbacRepo repository.RbacRepository,
	permissions PermissionChecker,
	emailService email.EmailService,
) InvitationUsecase {
	return &invitationUsecase{
		invitationRepo: invitationRepo,
		tenantRepo:     tenantRepo,
		userRepo:       userRepo,
		rbacRepo:       rbacRepo,
		permissions:    permissions,
		emailService:   emailService,
	}
}

func (u *invitationUsecase) CreateInvitation(ctx context.Context, tenantID, callerRoleID, roleID uuid.UUID, emailAddr string) (*domain.TenantInvitation, error) {
	if err := requirePermission(ctx, u.permissions, tenantID, callerRoleID, "member:invite"); err != nil {
		return nil, err
	}

	// GUARD: the invited role must belong to this tenant or be a system role, so an
	// invitation can never grant a role owned by another tenant.
	if err := u.validateInvitableRole(ctx, tenantID, roleID); err != nil {
		return nil, err
	}

	// Registration via invitation creates a new account; any existing account
	// would be unable to redeem the token. Do not send an unusable invitation.
	user, err := u.userRepo.GetByEmail(ctx, emailAddr)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}
	if user != nil {
		return nil, domain.ErrUserAlreadyExists
	}

	// 2. Fetch tenant to get tenant name
	tenant, err := u.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// 3. Generate invitation
	token := uuid.New().String()
	invitation := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		RoleID:    roleID,
		Email:     emailAddr,
		Token:     token,
		IsUsed:    false,
		ExpiresAt: time.Now().Add(48 * time.Hour),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 4. Insert into database
	if err := u.invitationRepo.ReplaceActive(ctx, invitation); err != nil {
		return nil, err
	}

	// 5. Send the invitation email. A delivery failure never fails the stored
	// invitation: the outcome is reported on the invitation and logged without
	// the token so the tenant can be told the email did not arrive (KEL-36).
	if err := u.emailService.SendInvitationEmail(emailAddr, token, tenant.Name); err != nil {
		invitation.EmailSent = false
		slog.ErrorContext(ctx, "invitation email delivery failed",
			"invitation_id", invitation.ID,
			"tenant_id", tenantID,
			"error", err,
		)
	} else {
		invitation.EmailSent = true
	}

	return invitation, nil
}

// validateInvitableRole rejects roles that neither belong to tenantID nor are system roles
// (tenant_id IS NULL). A missing role is reported with the same validation error so callers
// cannot distinguish "not found" from "belongs to another tenant".
func (u *invitationUsecase) validateInvitableRole(ctx context.Context, tenantID, roleID uuid.UUID) error {
	role, err := u.rbacRepo.GetRoleByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, domain.ErrRoleNotFound) {
			return domain.ErrInvitationRoleInvalid
		}
		return err
	}
	if role == nil {
		return domain.ErrInvitationRoleInvalid
	}
	if role.TenantID == nil {
		if role.Name == "Creator" {
			return domain.ErrCreatorGrantForbidden
		}
		return nil
	}
	if *role.TenantID != tenantID {
		return domain.ErrInvitationRoleInvalid
	}
	return nil
}

func (u *invitationUsecase) ListInvitations(ctx context.Context, tenantID, callerRoleID uuid.UUID) ([]domain.TenantInvitation, error) {
	if err := requirePermission(ctx, u.permissions, tenantID, callerRoleID, "member:invite"); err != nil {
		return nil, err
	}
	return u.invitationRepo.ListPending(ctx, tenantID)
}

func (u *invitationUsecase) RevokeInvitation(ctx context.Context, tenantID, callerRoleID, invitationID uuid.UUID) error {
	if err := requirePermission(ctx, u.permissions, tenantID, callerRoleID, "member:invite"); err != nil {
		return err
	}
	return u.invitationRepo.Revoke(ctx, tenantID, invitationID)
}

func (u *invitationUsecase) VerifyInvitation(ctx context.Context, token string) (*domain.TenantInvitation, error) {
	invitation, err := u.invitationRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if invitation.IsUsed {
		return nil, domain.ErrInvitationUsed
	}

	if time.Now().After(invitation.ExpiresAt) {
		return nil, domain.ErrInvitationExpired
	}

	return invitation, nil
}
