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

	// 1. Check if user already exists and is a member of this tenant
	user, err := u.userRepo.GetByEmail(ctx, emailAddr)
	if err == nil && user != nil {
		member, err := u.userRepo.GetTenantMemberByUserID(ctx, user.ID)
		if err == nil && member != nil && member.TenantID == tenantID {
			return nil, domain.ErrAlreadyTenantMember
		}
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
	if err := u.invitationRepo.Create(ctx, invitation); err != nil {
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
	if role == nil || role.TenantID == nil {
		// System roles are valid for every tenant.
		return nil
	}
	if *role.TenantID != tenantID {
		return domain.ErrInvitationRoleInvalid
	}
	return nil
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
