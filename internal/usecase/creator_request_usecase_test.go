package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type creatorRequestStub struct {
	calls                   int
	email, reason           string
	tenant, requester, role uuid.UUID
	target                  *uuid.UUID
	items                   []domain.CreatorRequest
	err                     error
}

func (s *creatorRequestStub) Create(_ context.Context, tenant, requester, role uuid.UUID, email string, target *uuid.UUID, reason string) (*domain.CreatorRequest, error) {
	s.calls++
	s.tenant, s.requester, s.role, s.email, s.target, s.reason = tenant, requester, role, email, target, reason
	if s.err != nil {
		return nil, s.err
	}
	return &domain.CreatorRequest{TenantID: tenant, TargetEmail: email, Status: "pending"}, nil
}
func (s *creatorRequestStub) List(_ context.Context, tenant, requester, role uuid.UUID) ([]domain.CreatorRequest, error) {
	s.calls++
	s.tenant, s.requester, s.role = tenant, requester, role
	return s.items, s.err
}
func TestCreatorRequestUsecaseValidationAndScope(t *testing.T) {
	tenant, user, role, target := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, tc := range []struct {
		name, email, reason string
		target              *uuid.UUID
		tenant, user, role  uuid.UUID
		want                error
	}{
		{name: "missing target", reason: "why", tenant: tenant, user: user, role: role, want: domain.ErrCreatorRequestInvalid},
		{name: "bad email", email: "invalid", reason: "why", tenant: tenant, user: user, role: role, want: domain.ErrCreatorRequestInvalid},
		{name: "blank reason", email: "a@example.com", reason: "  ", tenant: tenant, user: user, role: role, want: domain.ErrCreatorRequestInvalid},
		{name: "no tenant", email: "a@example.com", reason: "why", user: user, role: role, want: domain.ErrCreatorRequestForbidden},
		{name: "no user", email: "a@example.com", reason: "why", tenant: tenant, role: role, want: domain.ErrCreatorRequestForbidden},
		{name: "no role", email: "a@example.com", reason: "why", tenant: tenant, user: user, want: domain.ErrCreatorRequestForbidden},
		{name: "valid email", email: " A@EXAMPLE.COM ", reason: " why ", tenant: tenant, user: user, role: role},
		{name: "valid user", target: &target, reason: "why", tenant: tenant, user: user, role: role},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &creatorRequestStub{}
			result, err := NewCreatorRequestUsecase(stub).Create(context.Background(), tc.tenant, tc.user, tc.role, tc.email, tc.target, tc.reason)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want %v", err, tc.want)
			}
			if tc.want != nil && stub.calls != 0 {
				t.Fatal("invalid request reached repository")
			}
			if tc.want == nil && (result.Status != "pending" || stub.calls != 1 || stub.tenant != tenant || stub.requester != user || stub.role != role) {
				t.Fatalf("unexpected request: %+v %+v", result, stub)
			}
			if tc.name == "valid email" && (stub.email != "a@example.com" || stub.reason != "why") {
				t.Fatalf("not normalized: %+v", stub)
			}
		})
	}
	stub := &creatorRequestStub{items: []domain.CreatorRequest{{TenantID: tenant, Status: "pending"}}}
	if _, err := NewCreatorRequestUsecase(stub).List(context.Background(), tenant, user, role); err != nil || stub.calls != 1 || stub.tenant != tenant {
		t.Fatalf("list: %v %+v", err, stub)
	}
	if _, err := NewCreatorRequestUsecase(stub).List(context.Background(), uuid.Nil, user, role); !errors.Is(err, domain.ErrCreatorRequestForbidden) || stub.calls != 1 {
		t.Fatalf("invalid list: %v", err)
	}
}
