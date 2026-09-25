package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type platformUsers struct{ user *domain.User }

func (u platformUsers) Create(context.Context, *domain.User) error { panic("unused") }
func (u platformUsers) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u.user != nil && u.user.ID == id {
		return u.user, nil
	}
	return nil, domain.ErrUserNotFound
}
func (u platformUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if u.user != nil && u.user.Email == email {
		return u.user, nil
	}
	return nil, domain.ErrUserNotFound
}
func (u platformUsers) Update(context.Context, *domain.User) error { panic("unused") }
func (u platformUsers) Delete(context.Context, uuid.UUID) error    { panic("unused") }
func (u platformUsers) RegisterTenantTx(context.Context, *domain.User, *domain.Tenant) (*domain.TenantMember, error) {
	panic("unused")
}
func (u platformUsers) RegisterInvitedUserTx(context.Context, string, string, string, string) (*domain.User, error) {
	panic("unused")
}
func (u platformUsers) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	panic("unused")
}
func (u platformUsers) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	panic("unused")
}

type platformAssignment struct {
	active bool
	err    error
}

func (a *platformAssignment) IsActive(context.Context, uuid.UUID) (bool, error) {
	return a.active, a.err
}

func TestPlatformLoginAndRevocation(t *testing.T) {
	hashed, err := hash.HashPassword("valid-password")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{ID: uuid.New(), Email: "admin@example.test", PasswordHash: hashed}
	assignment := &platformAssignment{active: true}
	tokens := jwt.NewJWTService("test-secret", time.Hour)
	store := &factorMemory{id: user.ID, version: 1, enrolled: true}
	auth := NewPlatformAuth(platformUsers{user}, assignment, tokens).WithFactor(store)
	token, err := auth.Login(context.Background(), user.Email, "valid-password")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.ValidateToken(token)
	if err != nil || !claims.PlatformPending || claims.IsPlatformAdmin || claims.TenantID != uuid.Nil || claims.RoleID != uuid.Nil || claims.IsParent {
		t.Fatalf("invalid platform principal: %+v %v", claims, err)
	}
	if err := auth.Check(context.Background(), claims.UserID); err != nil {
		t.Fatal(err)
	}
	assignment.active = false
	if err := auth.Check(context.Background(), claims.UserID); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("revocation: %v", err)
	}
	if _, err := auth.Login(context.Background(), user.Email, "valid-password"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("inactive login: %v", err)
	}
	assignment.active = true
	if err := auth.Check(context.Background(), claims.UserID); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	assignment.err = errors.New("database offline")
	if err := auth.Check(context.Background(), claims.UserID); err == nil || errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("database failure should not allow or mask: %v", err)
	}
	if _, err := auth.Login(context.Background(), user.Email, "bad password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("password: %v", err)
	}
	if _, err := auth.Login(context.Background(), "missing@example.test", "valid-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("missing user: %v", err)
	}
}
