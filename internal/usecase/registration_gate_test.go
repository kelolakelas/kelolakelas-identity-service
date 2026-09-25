package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

// registrationGateUserStub records RegisterTenantTx calls so the gate tests
// can prove that a closed or undecidable policy never reaches the transaction.
type registrationGateUserStub struct {
	txMu       sync.Mutex
	txCalls    int
	txErr      error
	memberRole uuid.UUID
}

func (s *registrationGateUserStub) Create(context.Context, *domain.User) error { return nil }
func (s *registrationGateUserStub) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (s *registrationGateUserStub) GetByEmail(context.Context, string) (*domain.User, error) {
	return nil, domain.ErrUserNotFound
}
func (s *registrationGateUserStub) Update(context.Context, *domain.User) error { return nil }
func (s *registrationGateUserStub) Delete(context.Context, uuid.UUID) error    { return nil }
func (s *registrationGateUserStub) RegisterInvitedUserTx(context.Context, string, string, string, string) (*domain.User, error) {
	return nil, nil
}
func (s *registrationGateUserStub) GetPermissionsByRoleId(context.Context, uuid.UUID) ([]string, error) {
	return nil, nil
}
func (s *registrationGateUserStub) GetTenantMemberByUserID(context.Context, uuid.UUID) (*domain.TenantMember, error) {
	return nil, nil
}
func (s *registrationGateUserStub) RegisterTenantTx(_ context.Context, _ *domain.User, _ *domain.Tenant) (*domain.TenantMember, error) {
	s.txMu.Lock()
	s.txCalls++
	s.txMu.Unlock()
	if s.txErr != nil {
		return nil, s.txErr
	}
	return &domain.TenantMember{ID: uuid.New(), RoleID: s.memberRole, IsActive: true}, nil
}

type registrationGateTenantStub struct{}

func (s *registrationGateTenantStub) Create(context.Context, *domain.Tenant) error { return nil }
func (s *registrationGateTenantStub) GetByID(context.Context, uuid.UUID) (*domain.Tenant, error) {
	return &domain.Tenant{ID: uuid.New(), Name: "Existing", Status: "active"}, nil
}
func (s *registrationGateTenantStub) IsNameExists(context.Context, string) (bool, error) {
	return false, nil
}
func (s *registrationGateTenantStub) IsNameExistsExcept(context.Context, string, uuid.UUID) (bool, error) {
	return false, nil
}
func (s *registrationGateTenantStub) Update(context.Context, *domain.Tenant) error { return nil }
func (s *registrationGateTenantStub) Delete(context.Context, uuid.UUID) error      { return nil }

// fixedRegistrationPolicy returns a canned evaluation, optionally with an error
// that models an unavailable configuration store.
type fixedRegistrationPolicy struct {
	evaluation domain.RegistrationPolicyEvaluated
	err        error
}

func (p *fixedRegistrationPolicy) Evaluate(context.Context) (domain.RegistrationPolicyEvaluated, error) {
	return p.evaluation, p.err
}
func (p *fixedRegistrationPolicy) Close(_ context.Context, _ uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	return p.evaluation, p.err
}
func (p *fixedRegistrationPolicy) Open(_ context.Context, _ uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	return p.evaluation, p.err
}

func newRegistrationGateUsecase(users *registrationGateUserStub, policy domain.RegistrationPolicy) domain.TenantUsecase {
	return NewTenantUsecaseWithRegistrationPolicy(
		users, &registrationGateTenantStub{}, &permissionCheckerStub{},
		jwt.NewJWTService("registration-gate-test-secret", time.Hour), nil, nil, policy,
	)
}

func registrationRequest() *domain.RegisterTenantRequest {
	return &domain.RegisterTenantRequest{
		Email: "owner@example.com", Password: "password", FirstName: "First", LastName: "Last",
		TenantName: "Fresh Tenant",
	}
}

func TestRegisterTenantHonoursRegistrationPolicy(t *testing.T) {
	tests := []struct {
		name      string
		policy    domain.RegistrationPolicy
		wantError error
	}{
		{
			name:      "missing policy fails closed",
			wantError: domain.ErrRegistrationClosed,
		},
		{
			name:   "open policy registers",
			policy: &fixedRegistrationPolicy{evaluation: domain.RegistrationPolicyEvaluated{Open: true, AppliedVersion: 1, DesiredVersion: 1}},
		},
		{
			name:      "closed policy rejects with stable domain error",
			policy:    &fixedRegistrationPolicy{evaluation: domain.RegistrationPolicyEvaluated{Open: false, AppliedVersion: 2, DesiredVersion: 2}},
			wantError: domain.ErrRegistrationClosed,
		},
		{
			name:      "configuration store outage fails closed",
			policy:    &fixedRegistrationPolicy{err: errRegistrationStoreOutage},
			wantError: domain.ErrRegistrationClosed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			users := &registrationGateUserStub{memberRole: uuid.New()}
			usecase := newRegistrationGateUsecase(users, tc.policy)
			response, err := usecase.RegisterTenant(context.Background(), registrationRequest())
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) {
					t.Fatalf("err=%v want=%v", err, tc.wantError)
				}
				if response != nil {
					t.Fatalf("rejected registration returned response %+v", response)
				}
			} else if err != nil {
				t.Fatalf("register: %v", err)
			}
			users.txMu.Lock()
			calls := users.txCalls
			users.txMu.Unlock()
			if tc.wantError != nil {
				if calls != 0 {
					t.Fatalf("rejected registration reached the transaction %d times", calls)
				}
				return
			}
			if calls != 1 {
				t.Fatalf("accepted registration reached the transaction %d times", calls)
			}
		})
	}
}

// errRegistrationStoreOutage stands in for any policy read failure: the gate
// must reject registration when the effective value cannot be determined.
var errRegistrationStoreOutage = errors.New("configuration store unavailable")

// concurrentRegistrationPolicy flips to closed after the configured number of
// Evaluate calls, modelling an admin closing registration while registrations
// are in flight.
type concurrentRegistrationPolicy struct {
	mu           sync.Mutex
	evaluations  int
	closedAfter  int
	flipDuration time.Duration
}

func (p *concurrentRegistrationPolicy) Evaluate(context.Context) (domain.RegistrationPolicyEvaluated, error) {
	p.mu.Lock()
	p.evaluations++
	closed := p.evaluations > p.closedAfter
	p.mu.Unlock()
	if p.flipDuration > 0 {
		time.Sleep(p.flipDuration)
	}
	return domain.RegistrationPolicyEvaluated{Open: !closed}, nil
}
func (p *concurrentRegistrationPolicy) Close(_ context.Context, _ uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return domain.RegistrationPolicyEvaluated{Open: false}, nil
}
func (p *concurrentRegistrationPolicy) Open(_ context.Context, _ uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return domain.RegistrationPolicyEvaluated{Open: true}, nil
}

func TestRegisterTenantRaceAgainstPolicyClose(t *testing.T) {
	for _, tc := range []struct {
		name        string
		closedAfter int
		wantTx      int
	}{
		{name: "close before any registration consults the policy", closedAfter: 0, wantTx: 0},
		{name: "first registration passes the outer check and wins the race", closedAfter: 1, wantTx: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			users := &registrationGateUserStub{memberRole: uuid.New()}
			policy := &concurrentRegistrationPolicy{closedAfter: tc.closedAfter}
			usecase := newRegistrationGateUsecase(users, policy)
			_, err := usecase.RegisterTenant(context.Background(), registrationRequest())
			if tc.wantTx == 0 {
				if !errors.Is(err, domain.ErrRegistrationClosed) {
					t.Fatalf("err=%v want ErrRegistrationClosed", err)
				}
			} else if err != nil {
				t.Fatalf("register: %v", err)
			}
			users.txMu.Lock()
			calls := users.txCalls
			users.txMu.Unlock()
			if calls != tc.wantTx {
				t.Fatalf("tx calls=%d want %d", calls, tc.wantTx)
			}
		})
	}
}
