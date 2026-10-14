package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func TestAdminReadPermissionDenial(t *testing.T) {
	tenant, role, user, member := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, permission := range []string{"member:read", "role:read", "tenant:read"} {
		t.Run(permission, func(t *testing.T) {
			for _, lookupError := range []error{nil, errors.New("database unavailable")} {
				stub := &memberRepositoryStub{err: lookupError}
				ctx := domain.WithCaller(context.Background(), domain.Caller{UserID: user, MemberID: member, RoleID: role})
				var err error
				switch permission {
				case "member:read":
					_, err = NewMemberUsecase(stub).List(ctx, tenant, domain.MemberQuery{})
					if err == nil {
						t.Fatal("list bypassed permission")
					}
					_, err = NewMemberUsecase(stub).GetByID(ctx, tenant, uuid.New())
				case "role:read":
					_, err = NewRoleUsecase(nil, stub).FetchTenantRoles(ctx, tenant)
				case "tenant:read":
					_, err = NewTenantUsecase(nil, nil, stub, nil, nil, nil).GetTenantSettings(ctx, tenant)
				}
				want := lookupError
				if want == nil {
					want = domain.ErrPermissionDenied
				}
				if !errors.Is(err, want) {
					t.Fatalf("error = %v, want %v", err, want)
				}
				wantQuery := domain.MemberPermissionQuery{TenantID: tenant, RoleID: role, UserID: user, MemberID: member, Permission: permission}
				if stub.permissionQuery != wantQuery {
					t.Fatalf("query = %+v", stub.permissionQuery)
				}
			}
		})
	}
}

func TestMemberReadAllowedAndMissingCaller(t *testing.T) {
	tenant, role, user := uuid.New(), uuid.New(), uuid.New()
	stub := &memberRepositoryStub{allowed: true}
	u := NewMemberUsecase(stub)
	ctx := domain.WithCaller(context.Background(), domain.Caller{UserID: user, RoleID: role})
	if _, err := u.List(ctx, tenant, domain.MemberQuery{}); err != nil {
		t.Fatal(err)
	}
	if _, err := u.GetByID(ctx, tenant, uuid.New()); err != nil {
		t.Fatal(err)
	}
	stub.permissionQuery = domain.MemberPermissionQuery{}
	if _, err := u.List(context.Background(), tenant, domain.MemberQuery{}); !errors.Is(err, domain.ErrPermissionDenied) {
		t.Fatal(err)
	}
	if stub.permissionQuery != (domain.MemberPermissionQuery{}) {
		t.Fatal("missing caller reached repository")
	}
}
