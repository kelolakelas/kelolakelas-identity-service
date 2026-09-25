package usecase

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type factorMemory struct {
	mu        sync.Mutex
	id        uuid.UUID
	version   int64
	enrolled  bool
	allowed   bool
	challenge []byte
	purpose   string
	secret    []byte
	expires   time.Time
	attempts  int
	used      bool
	failure   bool
}

func (m *factorMemory) AssignmentVersion(_ context.Context, id uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure {
		return 0, errors.New("offline")
	}
	if id != m.id {
		return 0, errors.New("unknown")
	}
	return m.version, nil
}
func (m *factorMemory) Version(_ context.Context, id uuid.UUID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure {
		return 0, errors.New("offline")
	}
	if id != m.id || !m.enrolled {
		return 0, errors.New("unenrolled")
	}
	return m.version, nil
}
func (m *factorMemory) Begin(_ context.Context, id uuid.UUID, purpose string, token, secret []byte, expires time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure {
		return 0, errors.New("offline")
	}
	if id != m.id || (purpose == "enroll" && (!m.allowed || m.enrolled)) || (purpose == "verify" && !m.enrolled) {
		return 0, errors.New("denied")
	}
	m.challenge = append([]byte{}, token...)
	m.purpose = purpose
	m.expires = expires
	m.used = false
	m.attempts = 0
	if purpose == "enroll" {
		m.secret = secret
	}
	return m.version, nil
}
func (m *factorMemory) Consume(_ context.Context, id uuid.UUID, purpose string, token []byte, now time.Time, verify func([]byte) (bool, error)) (bool, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure {
		return false, 0, errors.New("offline")
	}
	if id != m.id || purpose != m.purpose || hex.EncodeToString(token) != hex.EncodeToString(m.challenge) || m.used || !now.Before(m.expires) || m.attempts >= 5 {
		return false, 0, nil
	}
	ok, err := verify(m.secret)
	if err != nil {
		return false, 0, err
	}
	if !ok {
		m.attempts++
		return false, 0, nil
	}
	m.used = true
	if purpose == "enroll" {
		m.enrolled = true
		m.allowed = false
		m.version++
	}
	return true, m.version, nil
}

var _ domain.PlatformFactorStore = (*factorMemory)(nil)

func TestFactorLifecycleAndReplay(t *testing.T) {
	hashed, _ := hash.HashPassword("valid-password")
	id := uuid.New()
	user := &domain.User{ID: id, Email: "admin@example.test", PasswordHash: hashed}
	store := &factorMemory{id: id, version: 1, allowed: true}
	assign := &platformAssignment{active: true}
	tokens := jwt.NewJWTService("test-secret", time.Hour)
	auth := NewPlatformAuth(platformUsers{user}, assign, tokens).WithFactor(store)
	pending, err := auth.Login(context.Background(), user.Email, "valid-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auth.FinishFactor(context.Background(), pending, "bad", "000000", "verify"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("bad challenge: %v", err)
	}
	ch, setup, err := auth.StartFactor(context.Background(), pending, "enroll")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := decodeSetup(setup)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.FinishFactor(context.Background(), pending, ch, factorCode(secret, time.Now().Unix()/30), "enroll")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := tokens.ValidateToken(token)
	if !claims.IsPlatformAdmin || claims.PlatformFactorVersion != 2 || claims.TenantID != uuid.Nil {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if _, err = auth.FinishFactor(context.Background(), pending, ch, factorCode(secret, time.Now().Unix()/30), "enroll"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("replay: %v", err)
	}
	if _, _, err = auth.StartFactor(context.Background(), pending, "enroll"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("reenroll: %v", err)
	}
	other, _ := tokens.GeneratePendingPlatformToken(uuid.New(), "other@example.test", 1)
	if _, err = auth.FinishFactor(context.Background(), other, ch, "000000", "enroll"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("other account: %v", err)
	}
	pending, err = auth.Login(context.Background(), user.Email, "valid-password")
	if err != nil {
		t.Fatal(err)
	}
	ch, _, err = auth.StartFactor(context.Background(), pending, "verify")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = auth.FinishFactor(context.Background(), pending, ch, factorCode(secret, time.Now().Unix()/30), "enroll"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("wrong purpose: %v", err)
	}
	store.mu.Lock()
	store.expires = time.Now().Add(-time.Second)
	store.mu.Unlock()
	if _, err = auth.FinishFactor(context.Background(), pending, ch, factorCode(secret, time.Now().Unix()/30), "verify"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("expired challenge: %v", err)
	}
	ch, _, err = auth.StartFactor(context.Background(), pending, "verify")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err = auth.FinishFactor(context.Background(), pending, ch, "000000", "verify"); err == nil {
			t.Fatal("bad code accepted")
		}
	}
	if _, err = auth.FinishFactor(context.Background(), pending, ch, factorCode(secret, time.Now().Unix()/30), "verify"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("attempt limit: %v", err)
	}
	store.failure = true
	if _, _, err = auth.StartFactor(context.Background(), pending, "verify"); !errors.Is(err, ErrPlatformUnavailable) {
		t.Fatalf("storage failure: %v", err)
	}
	store.failure = false
	store.mu.Lock()
	store.version++
	store.enrolled = false
	store.allowed = true
	store.mu.Unlock()
	if err = auth.CheckVersion(context.Background(), id, 2); err == nil {
		t.Fatal("old session after recovery")
	}
	if _, _, err = auth.StartFactor(context.Background(), pending, "enroll"); !errors.Is(err, ErrPlatformForbidden) {
		t.Fatalf("old pending after recovery: %v", err)
	}
}
func decodeSetup(s string) ([]byte, error) {
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
}
