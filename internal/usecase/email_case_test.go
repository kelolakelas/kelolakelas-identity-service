package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

// caseFoldUsers mirrors the repository contract of KEL-89: GetByEmail matches
// LOWER(email), and Create rejects any address that collides case-insensitively
// (the uq_users_email_lower index). Stored values are kept verbatim so a
// legacy mixed-case row stays mixed-case, exactly as the migration leaves it.
type caseFoldUsers struct {
	domain.UserRepository
	users       []*domain.User
	lookups     []string
	lookupErr   error
	createCalls int
}

func (s *caseFoldUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	s.lookups = append(s.lookups, email)
	if s.lookupErr != nil {
		return nil, s.lookupErr
	}
	for _, u := range s.users {
		if strings.ToLower(u.Email) == email {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func (s *caseFoldUsers) Create(_ context.Context, user *domain.User) error {
	s.createCalls++
	for _, u := range s.users {
		if strings.EqualFold(u.Email, user.Email) {
			return domain.ErrUserAlreadyExists
		}
	}
	s.users = append(s.users, user)
	return nil
}

func (s *caseFoldUsers) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	return nil, domain.ErrUserNotFound
}

func (s *caseFoldUsers) SessionValidAfter(context.Context, uuid.UUID) (*time.Time, error) {
	return nil, nil
}

func TestRegisterRejectsEmailDifferingOnlyByCase(t *testing.T) {
	users := &caseFoldUsers{users: []*domain.User{{ID: uuid.New(), Email: "owner@example.com"}}}
	auth := NewAuthUsecase(users, jwt.NewJWTService("secret", time.Hour), nil)
	for _, email := range []string{"Owner@Example.com", "OWNER@EXAMPLE.COM", "  owner@example.com "} {
		if _, err := auth.Register(context.Background(), &domain.User{Email: email}, "password"); !errors.Is(err, domain.ErrUserAlreadyExists) {
			t.Fatalf("register %q: %v", email, err)
		}
	}
	if users.createCalls != 0 || len(users.users) != 1 {
		t.Fatalf("duplicate reached Create: calls=%d users=%d", users.createCalls, len(users.users))
	}
}

func TestRegisterStoresCanonicalEmail(t *testing.T) {
	users := &caseFoldUsers{}
	auth := NewAuthUsecase(users, jwt.NewJWTService("secret", time.Hour), nil)
	created, err := auth.Register(context.Background(), &domain.User{Email: " New.User@Example.COM "}, "password")
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "new.user@example.com" || users.users[0].Email != "new.user@example.com" {
		t.Fatalf("stored email %q", users.users[0].Email)
	}
	if users.lookups[0] != "new.user@example.com" {
		t.Fatalf("lookup used %q", users.lookups[0])
	}
}

// The advisory lookup can race with a concurrent registration; the index is the
// final arbiter and its conflict must still surface as ErrUserAlreadyExists.
func TestRegisterConflictFromCreateIsReported(t *testing.T) {
	users := &raceUsers{caseFoldUsers: caseFoldUsers{}}
	auth := NewAuthUsecase(users, jwt.NewJWTService("secret", time.Hour), nil)
	if _, err := auth.Register(context.Background(), &domain.User{Email: "Racer@example.com"}, "password"); !errors.Is(err, domain.ErrUserAlreadyExists) {
		t.Fatalf("race: %v", err)
	}
}

type raceUsers struct{ caseFoldUsers }

func (s *raceUsers) Create(context.Context, *domain.User) error { return domain.ErrUserAlreadyExists }

func TestRegisterDoesNotTreatLookupFailureAsAvailable(t *testing.T) {
	users := &caseFoldUsers{lookupErr: errors.New("database unavailable")}
	auth := NewAuthUsecase(users, jwt.NewJWTService("secret", time.Hour), nil)
	if _, err := auth.Register(context.Background(), &domain.User{Email: "a@example.com"}, "password"); err == nil || errors.Is(err, domain.ErrUserAlreadyExists) {
		t.Fatalf("lookup failure: %v", err)
	}
	if users.createCalls != 0 {
		t.Fatal("created an account without a successful uniqueness check")
	}
}

func TestLoginMatchesAnyCaseVariantIncludingLegacyRows(t *testing.T) {
	passwordHash, err := hash.HashPassword("correct")
	if err != nil {
		t.Fatal(err)
	}
	// Stored before KEL-89 and intentionally left mixed-case by the migration.
	legacy := &domain.User{ID: uuid.New(), Email: "Legacy.User@Example.com", PasswordHash: passwordHash}
	users := &caseFoldUsers{users: []*domain.User{legacy}}
	auth := NewAuthUsecase(users, jwt.NewJWTService("secret", time.Hour), nil)
	for _, email := range []string{"legacy.user@example.com", "LEGACY.USER@EXAMPLE.COM", " Legacy.User@Example.com "} {
		token, got, _, err := auth.Login(context.Background(), email, "correct")
		if err != nil || got.ID != legacy.ID {
			t.Fatalf("login %q: %v", email, err)
		}
		claims, err := auth.jwtService.ValidateToken(token)
		if err != nil || claims.Email != "legacy.user@example.com" {
			t.Fatalf("login %q claim: %v %v", email, claims, err)
		}
	}
	if _, _, _, err := auth.Login(context.Background(), "LEGACY.USER@EXAMPLE.COM", "wrong"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, _, _, err := auth.Login(context.Background(), "other@example.com", "correct"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown email: %v", err)
	}
}

func TestLoginProtectionReceivesCanonicalEmail(t *testing.T) {
	store := &loginStoreEmailSpy{}
	auth := NewAuthUsecase(&caseFoldUsers{}, jwt.NewJWTService("secret", time.Hour), nil)
	if err := auth.WithLoginProtection(store, 3, time.Minute); err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = auth.Login(context.Background(), "  Mixed@Example.COM", "password")
	if store.email != "mixed@example.com" {
		t.Fatalf("login store got %q", store.email)
	}
}

type loginStoreEmailSpy struct{ email string }

func (s *loginStoreEmailSpy) Authenticate(_ context.Context, email, _, _ string, _ int, _ time.Duration) (*domain.User, error) {
	s.email = email
	return nil, domain.ErrInvalidCredentials
}

func TestTenantRegistrationRejectsCaseVariantEmail(t *testing.T) {
	users := &tenantCaseFoldUsers{caseFoldUsers: caseFoldUsers{users: []*domain.User{{ID: uuid.New(), Email: "Owner@Example.com"}}}}
	tenantUC := NewTenantUsecaseWithRegistrationPolicy(users, &registrationGateTenantStub{}, &permissionCheckerStub{},
		jwt.NewJWTService("secret", time.Hour), nil, nil, &fixedRegistrationPolicy{evaluation: domain.RegistrationPolicyEvaluated{Open: true}})
	req := &domain.RegisterTenantRequest{Email: "OWNER@example.COM", Password: "password", FirstName: "A", LastName: "B", TenantName: "kel89-tenant"}
	if _, err := tenantUC.RegisterTenant(context.Background(), req); !errors.Is(err, domain.ErrUserAlreadyExists) {
		t.Fatalf("tenant register: %v", err)
	}
	if users.txUser != nil {
		t.Fatal("duplicate reached RegisterTenantTx")
	}
	req.Email = " Fresh.Owner@Example.COM"
	if _, err := tenantUC.RegisterTenant(context.Background(), req); err != nil {
		t.Fatalf("fresh tenant register: %v", err)
	}
	if users.txUser == nil || users.txUser.Email != "fresh.owner@example.com" {
		t.Fatalf("tenant owner stored as %+v", users.txUser)
	}
}

type tenantCaseFoldUsers struct {
	caseFoldUsers
	txUser *domain.User
}

func (s *tenantCaseFoldUsers) RegisterTenantTx(_ context.Context, user *domain.User, _ *domain.Tenant) (*domain.TenantMember, error) {
	s.txUser = user
	return &domain.TenantMember{ID: uuid.New(), IsActive: true}, nil
}

func (s *tenantCaseFoldUsers) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}
