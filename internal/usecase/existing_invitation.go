package usecase

import (
	"context"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type ExistingInvitationStore interface {
	AcceptExisting(context.Context, string, uuid.UUID, func(*domain.User) error) (*domain.TenantMember, error)
}
type ExistingInvitations struct {
	store  ExistingInvitationStore
	tokens *jwt.JWTService
}

func NewExistingInvitations(store ExistingInvitationStore, tokens *jwt.JWTService) *ExistingInvitations {
	return &ExistingInvitations{store, tokens}
}
func (s *ExistingInvitations) Accept(ctx context.Context, token, accessToken string) (*domain.TenantMember, error) {
	sessions := TenantSessions{tokens: s.tokens}
	claims, err := sessions.principal(accessToken)
	if err != nil {
		return nil, err
	}
	return s.store.AcceptExisting(ctx, token, claims.UserID, func(user *domain.User) error { return validTenantUser(user, claims) })
}
