package usecase

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/email"
)

type CreatorDecisionUsecase struct {
	repo  repository.CreatorDecisionRepository
	email email.EmailService
}

func NewCreatorDecisionUsecase(repo repository.CreatorDecisionRepository, mail email.EmailService) *CreatorDecisionUsecase {
	return &CreatorDecisionUsecase{repo: repo, email: mail}
}
func (u *CreatorDecisionUsecase) Decide(ctx context.Context, requestID, actorID uuid.UUID, approve bool, reason string) (*domain.CreatorRequest, error) {
	if requestID == uuid.Nil || actorID == uuid.Nil {
		return nil, domain.ErrCreatorRequestInvalid
	}
	request, invitation, tenantName, err := u.repo.Decide(ctx, requestID, actorID, approve, reason)
	if err != nil {
		return nil, err
	}
	if invitation != nil {
		if err := u.email.SendInvitationEmail(invitation.Email, invitation.Token, tenantName); err != nil {
			slog.ErrorContext(ctx, "creator invitation delivery failed", "request_id", requestID, "invitation_id", invitation.ID, "error", err)
		}
	}
	return request, nil
}
