package usecase

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
	"testing"
	"time"
)

type failingMembershipUsers struct{ resetUsers }

func (s failingMembershipUsers) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	return nil, errors.New("database unavailable")
}
func TestTenantLoginFailsClosedOnMembershipReadFailureAndParentRemainsTenantless(t *testing.T) {
	password, _ := hash.HashPassword("correct")
	user := &domain.User{ID: uuid.New(), Email: "fixture@example.test", PasswordHash: password}
	auth := NewAuthUsecase(failingMembershipUsers{resetUsers{user}}, jwt.NewJWTService("secret", time.Hour), nil)
	if token, _, _, err := auth.Login(context.Background(), user.Email, "correct"); err == nil || token != "" {
		t.Fatal("issued token without a successful membership read")
	}
	user.IsParent = true
	token, _, tenant, err := auth.Login(context.Background(), user.Email, "correct")
	if err != nil || tenant != uuid.Nil {
		t.Fatalf("parent affected: %v", err)
	}
	claims, err := auth.jwtService.ValidateToken(token)
	if err != nil || !claims.IsParent || claims.MemberID != uuid.Nil || claims.RoleID != uuid.Nil {
		t.Fatalf("parent gained tenant claims: %v", err)
	}
}
