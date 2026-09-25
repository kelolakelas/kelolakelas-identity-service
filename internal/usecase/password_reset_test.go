package usecase

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type resetUsers struct{ user *domain.User }

func (s resetUsers) Create(context.Context, *domain.User) error               { return nil }
func (s resetUsers) GetByID(context.Context, uuid.UUID) (*domain.User, error) { return s.user, nil }
func (s resetUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	if email != s.user.Email {
		return nil, domain.ErrUserNotFound
	}
	return s.user, nil
}
func (s resetUsers) Update(context.Context, *domain.User) error { return nil }
func (s resetUsers) Delete(context.Context, uuid.UUID) error    { return nil }
func (s resetUsers) RegisterTenantTx(context.Context, *domain.User, *domain.Tenant) (*domain.TenantMember, error) {
	return nil, nil
}
func (s resetUsers) RegisterInvitedUserTx(context.Context, string, string, string, string) (*domain.User, error) {
	return nil, nil
}
func (s resetUsers) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s resetUsers) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	return nil, nil
}

type resetSender struct {
	token string
	err   error
}

func (s *resetSender) SendPasswordResetEmail(_ string, token string) error {
	s.token = token
	return s.err
}

type resetStore struct {
	mu       sync.Mutex
	token    string
	user     *domain.User
	expiry   time.Time
	used     bool
	boundary *time.Time
}

func (s *resetStore) Issue(_ context.Context, _ uuid.UUID, token string, expiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	s.expiry = expiry
	s.used = false
	return nil
}
func (s *resetStore) Consume(_ context.Context, token, password string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used || token != s.token || !now.Before(s.expiry) {
		return domain.ErrInvalidResetToken
	}
	s.used = true
	s.user.PasswordHash = password
	b := now.Truncate(time.Second).Add(time.Second)
	s.boundary = &b
	return nil
}
func (s *resetStore) SessionValidAfter(context.Context, uuid.UUID) (*time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.boundary, nil
}
func TestPasswordResetDeliveryFailureIsLoggedWithoutAddressOrToken(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	defer slog.SetDefault(previous)

	user := &domain.User{ID: uuid.New(), Email: "private@example.com"}
	store := &resetStore{user: user}
	sender := &resetSender{err: errors.New("mail transport unavailable")}
	u := NewAuthUsecase(resetUsers{user}, jwt.NewJWTService("secret", time.Hour), nil).WithPasswordReset(store, sender, time.Hour)
	if err := u.RequestPasswordReset(context.Background(), user.Email); err != nil {
		t.Fatal(err)
	}
	log := output.String()
	if !strings.Contains(log, "password reset email delivery failed") || !strings.Contains(log, "mail transport unavailable") {
		t.Fatalf("delivery failure not logged: %s", log)
	}
	if strings.Contains(log, user.Email) || strings.Contains(log, sender.token) {
		t.Fatal("reset log exposed address or token")
	}
}

func TestPasswordResetRotationSingleUseAndLogin(t *testing.T) {
	ctx := context.Background()
	initial, _ := hash.HashPassword("oldpass")
	user := &domain.User{ID: uuid.New(), Email: "a@example.com", PasswordHash: initial, IsParent: true}
	store := &resetStore{user: user}
	sender := &resetSender{err: errors.New("delivery failed")}
	u := NewAuthUsecase(resetUsers{user}, jwt.NewJWTService("secret", time.Hour), nil).WithPasswordReset(store, sender, time.Hour)
	if err := u.RequestPasswordReset(ctx, user.Email); err != nil {
		t.Fatal(err)
	}
	first := sender.token
	if first == "" || first == store.token || resetTokenHash(first) != store.token || store.expiry.Before(time.Now().Add(59*time.Minute)) {
		t.Fatal("token hash or TTL invalid")
	}
	if err := u.RequestPasswordReset(ctx, user.Email); err != nil {
		t.Fatal(err)
	}
	second := sender.token
	if err := u.ConfirmPasswordReset(ctx, first, "newpass"); !errors.Is(err, domain.ErrInvalidResetToken) {
		t.Fatalf("rotated token accepted: %v", err)
	}
	if err := u.RequestPasswordReset(ctx, "absent@example.com"); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := u.Login(ctx, user.Email, "oldpass")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- u.ConfirmPasswordReset(ctx, second, "newpass") }()
	}
	wg.Wait()
	close(results)
	success, invalid := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrInvalidResetToken) {
			invalid++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || invalid != 1 {
		t.Fatalf("success=%d invalid=%d", success, invalid)
	}
	if _, _, _, err := u.Login(ctx, user.Email, "oldpass"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("old password accepted: %v", err)
	}
	token, _, _, err := u.Login(ctx, user.Email, "newpass")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := u.jwtService.ValidateToken(token)
	if err != nil || claims.IssuedAt.Time.Before(*store.boundary) {
		t.Fatalf("new token invalid: %v", err)
	}
	store.expiry = time.Now().Add(-time.Second)
	store.used = false
	if err := u.ConfirmPasswordReset(ctx, second, "another"); !errors.Is(err, domain.ErrInvalidResetToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}
