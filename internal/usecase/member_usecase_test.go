package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type memberRepositoryStub struct {
	items            []domain.MemberResponse
	total            int64
	err              error
	query            domain.MemberQuery
	allowed          bool
	deleted          bool
	permissionTenant uuid.UUID
	permissionQuery  domain.MemberPermissionQuery
}

func (s *memberRepositoryStub) List(_ context.Context, _ uuid.UUID, query domain.MemberQuery) ([]domain.MemberResponse, int64, error) {
	s.query = query
	return s.items, s.total, s.err
}

func (s *memberRepositoryStub) GetByID(context.Context, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	return nil, s.err
}

func (s *memberRepositoryStub) UpdateRole(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	return nil, s.err
}

func (s *memberRepositoryStub) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	s.deleted = true
	return s.err
}

func (s *memberRepositoryStub) HasActiveMemberPermission(_ context.Context, query domain.MemberPermissionQuery) (bool, error) {
	s.permissionTenant = query.TenantID
	s.permissionQuery = query
	return s.allowed, s.err
}

func (s *memberRepositoryStub) ListTutors(context.Context, uuid.UUID, domain.TutorQuery) ([]domain.TutorResponse, int64, error) {
	return nil, 0, s.err
}

func TestMemberUsecaseDelete(t *testing.T) {
	tests := []struct {
		name          string
		allowed       bool
		err           error
		expectDeleted bool
		expectErr     error
	}{
		{name: "deletes with permission", allowed: true, expectDeleted: true},
		{name: "rejects without permission", expectErr: domain.ErrMemberDeletePermission},
		{name: "returns permission lookup error", err: errors.New("permission lookup failed"), expectErr: errors.New("permission lookup failed")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &memberRepositoryStub{allowed: test.allowed, err: test.err}
			tenantID := uuid.New()
			callerRoleID := uuid.New()
			err := NewMemberUsecase(stub).Delete(callerContext(), tenantID, callerRoleID, uuid.New())
			if test.expectErr != nil {
				if err == nil || err.Error() != test.expectErr.Error() {
					t.Fatalf("error = %v, want %v", err, test.expectErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stub.deleted != test.expectDeleted {
				t.Fatalf("deleted = %v, want %v", stub.deleted, test.expectDeleted)
			}
			if stub.permissionTenant != tenantID {
				t.Fatalf("permission tenant = %s, want %s", stub.permissionTenant, tenantID)
			}
			want := domain.MemberPermissionQuery{TenantID: tenantID, RoleID: callerRoleID, MemberID: testCaller.MemberID, UserID: testCaller.UserID, Permission: "member:delete"}
			if stub.permissionQuery != want {
				t.Fatalf("permission query = %+v, want %+v", stub.permissionQuery, want)
			}
		})
	}
}

// TestMemberUsecaseUpdateRoleRequiresCallerMembership proves member:update goes through the
// same KEL-76 membership rule as every other administrative check.
func TestMemberUsecaseUpdateRoleRequiresCallerMembership(t *testing.T) {
	tenantID, callerRoleID := uuid.New(), uuid.New()

	denied := &memberRepositoryStub{}
	if _, err := NewMemberUsecase(denied).UpdateRole(callerContext(), tenantID, callerRoleID, uuid.New(), uuid.New()); !errors.Is(err, domain.ErrMemberPermission) {
		t.Fatalf("error = %v, want ErrMemberPermission", err)
	}
	want := domain.MemberPermissionQuery{TenantID: tenantID, RoleID: callerRoleID, MemberID: testCaller.MemberID, UserID: testCaller.UserID, Permission: "member:update"}
	if denied.permissionQuery != want {
		t.Fatalf("permission query = %+v, want %+v", denied.permissionQuery, want)
	}

	noCaller := &memberRepositoryStub{allowed: true}
	if _, err := NewMemberUsecase(noCaller).UpdateRole(context.Background(), tenantID, callerRoleID, uuid.New(), uuid.New()); !errors.Is(err, domain.ErrMemberPermission) {
		t.Fatalf("error without caller = %v, want ErrMemberPermission", err)
	}
	if noCaller.permissionQuery != (domain.MemberPermissionQuery{}) {
		t.Fatalf("lookup made without a caller: %+v", noCaller.permissionQuery)
	}
}

func TestMemberUsecaseList(t *testing.T) {
	tests := []struct {
		name        string
		query       domain.MemberQuery
		total       int64
		expectPage  int
		expectSize  int
		expectPages int
		expectError bool
	}{
		{name: "defaults pagination", total: 21, expectPage: 1, expectSize: 20, expectPages: 2},
		{name: "normalizes invalid page size", query: domain.MemberQuery{Page: 0, PageSize: 101}, total: 0, expectPage: 1, expectSize: 20},
		{name: "returns repository error", expectError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &memberRepositoryStub{total: test.total}
			if test.expectError {
				stub.err = errors.New("database unavailable")
			}
			result, err := NewMemberUsecase(stub).List(context.Background(), uuid.New(), test.query)
			if test.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if stub.query.Page != test.expectPage || stub.query.PageSize != test.expectSize {
				t.Fatalf("pagination = %+v", stub.query)
			}
			if result.Pagination.TotalPages != test.expectPages {
				t.Fatalf("total pages = %d", result.Pagination.TotalPages)
			}
		})
	}
}
