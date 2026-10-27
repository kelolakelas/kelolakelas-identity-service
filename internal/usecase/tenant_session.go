package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type TenantSessions struct {
	store  domain.TenantSessionRepository
	tokens *jwt.JWTService
}

func NewTenantSessions(store domain.TenantSessionRepository, tokens *jwt.JWTService) *TenantSessions {
	return &TenantSessions{store: store, tokens: tokens}
}

func (s *TenantSessions) principal(token string) (*jwt.Claims, error) {
	c, err := s.tokens.ValidateToken(token)
	if err != nil || c.UserID == uuid.Nil || c.IssuedAt == nil {
		return nil, domain.ErrTenantSessionInvalid
	}
	if c.IsParent || c.IsPlatformAdmin || c.PlatformPending {
		return nil, domain.ErrTenantSessionForbidden
	}
	return c, nil
}

func validTenantUser(user *domain.User, c *jwt.Claims) error {
	if c.ExpiresAt == nil || !time.Now().Before(c.ExpiresAt.Time) {
		return domain.ErrTenantSessionInvalid
	}
	if user.SessionValidAfter != nil && c.IssuedAt.Time.Before(*user.SessionValidAfter) {
		return domain.ErrTenantSessionInvalid
	}
	if user.IsParent {
		return domain.ErrTenantSessionForbidden
	}
	return nil
}

func (s *TenantSessions) List(ctx context.Context, token string) ([]domain.TenantChoice, error) {
	c, err := s.principal(token)
	if err != nil {
		return nil, err
	}
	if err = s.store.WithUser(ctx, c.UserID, func(user *domain.User) error { return validTenantUser(user, c) }); err != nil {
		return nil, err
	}
	return s.store.List(ctx, c.UserID)
}

func (s *TenantSessions) Switch(ctx context.Context, token string, tenantID uuid.UUID) (string, *domain.TenantChoice, error) {
	c, err := s.principal(token)
	if err != nil {
		return "", nil, err
	}
	var result string
	var selected domain.TenantChoice
	err = s.store.WithMembership(ctx, c.UserID, tenantID, func(user *domain.User, choice domain.TenantChoice) error {
		if err := validTenantUser(user, c); err != nil {
			return err
		}
		var err error
		result, err = s.tokens.GenerateTokenAfter(user.ID, user.Email, choice.TenantID, choice.RoleID, choice.MemberID, false, user.SessionValidAfter)
		selected = choice
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return result, &selected, nil
}
