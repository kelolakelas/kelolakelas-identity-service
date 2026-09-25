package usecase

import (
	"context"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

type CreatorRequestUsecase struct {
	repo repository.CreatorRequestRepository
}

func NewCreatorRequestUsecase(repo repository.CreatorRequestRepository) *CreatorRequestUsecase {
	return &CreatorRequestUsecase{repo: repo}
}

func (u *CreatorRequestUsecase) Create(ctx context.Context, tenantID, requesterID, roleID uuid.UUID, email string, userID *uuid.UUID, reason string) (*domain.CreatorRequest, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	reason = strings.TrimSpace(reason)
	if tenantID == uuid.Nil || requesterID == uuid.Nil || roleID == uuid.Nil {
		return nil, domain.ErrCreatorRequestForbidden
	}
	if (email == "" && userID == nil) || (userID != nil && *userID == uuid.Nil) || len(reason) == 0 || len(reason) > 2000 || len(email) > 255 {
		return nil, domain.ErrCreatorRequestInvalid
	}
	if email != "" {
		parsed, err := mail.ParseAddress(email)
		if err != nil || parsed.Address != email {
			return nil, domain.ErrCreatorRequestInvalid
		}
	}
	return u.repo.Create(ctx, tenantID, requesterID, roleID, email, userID, reason)
}

func (u *CreatorRequestUsecase) List(ctx context.Context, tenantID, requesterID, roleID uuid.UUID) ([]domain.CreatorRequest, error) {
	if tenantID == uuid.Nil || requesterID == uuid.Nil || roleID == uuid.Nil {
		return nil, domain.ErrCreatorRequestForbidden
	}
	return u.repo.List(ctx, tenantID, requesterID, roleID)
}
