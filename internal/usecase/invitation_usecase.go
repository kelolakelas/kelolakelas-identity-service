package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tutorin-id/tutorin-identity-service/internal/domain"
	"github.com/tutorin-id/tutorin-identity-service/internal/repository"
	"github.com/tutorin-id/tutorin-identity-service/pkg/email"
)

type invitationUsecase struct {
	invitationRepo repository.InvitationRepository
	tenantRepo     repository.TenantRepository
	userRepo       domain.UserRepository
	emailService   email.EmailService
}

func NewInvitationUsecase(
	invitationRepo repository.InvitationRepository,
	tenantRepo repository.TenantRepository,
	userRepo domain.UserRepository,
	emailService email.EmailService,
) InvitationUsecase {
	return &invitationUsecase{
		invitationRepo: invitationRepo,
		tenantRepo:     tenantRepo,
		userRepo:       userRepo,
		emailService:   emailService,
	}
}

func (u *invitationUsecase) CreateInvitation(ctx context.Context, tenantID, roleID uuid.UUID, emailAddr string) (*domain.TenantInvitation, error) {
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

	// 5. Send invitation email
	_ = u.emailService.SendInvitationEmail(emailAddr, token, tenant.Name)

	return invitation, nil
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
