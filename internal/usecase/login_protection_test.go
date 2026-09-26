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

type loginStoreStub struct {
	user      *domain.User
	err       error
	threshold int
	duration  time.Duration
	dummy     string
}

func (s *loginStoreStub) Authenticate(_ context.Context, _, _, dummy string, threshold int, duration time.Duration) (*domain.User, error) {
	s.threshold, s.duration, s.dummy = threshold, duration, dummy
	return s.user, s.err
}

func TestLoginProtectionKeepsCredentialErrorAndToken(t *testing.T) {
	passwordHash, err := hash.HashPassword("correct")
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{ID: uuid.New(), Email: "user@example.com", PasswordHash: passwordHash, IsParent: true}
	store := &loginStoreStub{err: domain.ErrInvalidCredentials}
	auth := NewAuthUsecase(resetUsers{user}, jwt.NewJWTService("secret", time.Hour), nil)
	if err := auth.WithLoginProtection(store, 3, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := auth.Login(context.Background(), user.Email, "correct"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("locked account: %v", err)
	}
	if store.threshold != 3 || store.duration != time.Minute || store.dummy == "" || hash.CheckPasswordHash("correct", store.dummy) {
		t.Fatal("protection configuration/dummy hash incorrect")
	}
	store.err, store.user = nil, user
	token, got, tenant, err := auth.Login(context.Background(), user.Email, "correct")
	if err != nil || got.ID != user.ID || tenant != uuid.Nil {
		t.Fatalf("normal login: %v", err)
	}
	claims, err := auth.jwtService.ValidateToken(token)
	if err != nil || claims.UserID != user.ID {
		t.Fatalf("JWT changed: %v", err)
	}
	store.err = errors.New("database unavailable")
	if _, _, _, err := auth.Login(context.Background(), user.Email, "correct"); err == nil || errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("store failure: %v", err)
	}
}

func TestLoginProtectionRejectsInvalidSetup(t *testing.T) {
	auth := NewAuthUsecase(nil, nil, nil)
	if err := auth.WithLoginProtection(nil, 3, time.Minute); err == nil {
		t.Fatal("accepted missing store")
	}
	for _, tc := range []struct {
		store     *loginStoreStub
		threshold int
		duration  time.Duration
	}{
		{&loginStoreStub{}, 0, time.Minute}, {&loginStoreStub{}, 3, 0},
	} {
		if err := auth.WithLoginProtection(tc.store, tc.threshold, tc.duration); err == nil {
			t.Fatal("accepted invalid protection config")
		}
	}
}
