package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// KEL-79: PUT /members/:id/role moves a Teacher (or custom-role) member to another allowed
// role, refuses the caller's own membership with a conflict, and keeps the tenant scope, the
// Creator grant block (KEL-94) and the guard on a current Creator member. Every rejection
// must happen before the UPDATE, so the row stays unchanged.

type roleUpdateFixture struct {
	tenant, member, user, currentRole, targetRole uuid.UUID
}

func newRoleUpdateFixture() roleUpdateFixture {
	return roleUpdateFixture{tenant: uuid.New(), member: uuid.New(), user: uuid.New(), currentRole: uuid.New(), targetRole: uuid.New()}
}

func (f roleUpdateFixture) expectMember(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_members"`)).
		WithArgs(f.member, f.tenant, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "user_id", "role_id"}).AddRow(f.member, f.tenant, f.user, f.currentRole))
}

// expectTargetRole asserts the target role lookup is scoped to the tenant or system roles.
func (f roleUpdateFixture) expectTargetRole(mock sqlmock.Sqlmock, tenantID interface{}, name string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND (tenant_id = $2 OR tenant_id IS NULL)`)).
		WithArgs(f.targetRole, f.tenant, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}).AddRow(f.targetRole, tenantID, name))
}

func (f roleUpdateFixture) expectCurrentRole(mock sqlmock.Sqlmock, tenantID interface{}, name string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1`)).
		WithArgs(f.currentRole, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}).AddRow(f.currentRole, tenantID, name))
}

func (f roleUpdateFixture) expectUpdateAndRead(mock sqlmock.Sqlmock, roleName string, system bool) {
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "tenant_members" SET "role_id"=$1`)).
		WithArgs(f.targetRole, sqlmock.AnyArg(), f.member).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`FROM tenant_members tm JOIN users u ON u.id = tm.user_id JOIN roles ro ON ro.id = tm.role_id`)).
		WithArgs(f.tenant, f.member, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "tenant_id", "email", "first_name", "last_name", "phone", "status", "role_id", "role_name", "is_system_role", "created_at", "updated_at"}).
			AddRow(f.member, f.user, f.tenant, "teacher@example.com", "T", "Eacher", nil, "active", f.targetRole, roleName, system, now, now))
}

func otherActor() domain.Caller {
	return domain.Caller{UserID: uuid.New(), MemberID: uuid.New()}
}

func TestUpdateRoleMovesTeacherToTenantCustomRole(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newRoleUpdateFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	f.expectTargetRole(mock, f.tenant, "Staff")
	f.expectCurrentRole(mock, nil, "Teacher")
	f.expectUpdateAndRead(mock, "Staff", false)

	result, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, otherActor())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Role.ID != f.targetRole || result.Role.Name != "Staff" || result.Role.IsSystemRole {
		t.Fatalf("role = %+v, want tenant role Staff", result.Role)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateRoleCustomMemberToSystemTeacherStillAllowed keeps the pre-KEL-79 path working.
func TestUpdateRoleCustomMemberToSystemTeacherStillAllowed(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newRoleUpdateFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	f.expectTargetRole(mock, nil, "Teacher")
	f.expectCurrentRole(mock, f.tenant, "Staff")
	f.expectUpdateAndRead(mock, "Teacher", true)

	result, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, otherActor())
	if err != nil || result.Role.Name != "Teacher" || !result.Role.IsSystemRole {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateRoleRejectsOwnMembershipBeforeAnyWrite covers the self-target guard for tokens
// with a member_id claim and for legacy tokens identified only by user id.
func TestUpdateRoleRejectsOwnMembershipBeforeAnyWrite(t *testing.T) {
	f := newRoleUpdateFixture()
	actors := map[string]domain.Caller{
		"member_id claim":            {UserID: f.user, MemberID: f.member},
		"legacy token without claim": {UserID: f.user},
		"member_id matches only":     {UserID: uuid.New(), MemberID: f.member},
	}
	for name, actor := range actors {
		t.Run(name, func(t *testing.T) {
			repo, mock := newMemberRepositoryWithMock(t)
			mock.ExpectBegin()
			f.expectMember(mock)
			mock.ExpectRollback()

			result, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, actor)
			if !errors.Is(err, domain.ErrMemberSelfRoleChange) || result != nil {
				t.Fatalf("result=%+v err=%v, want ErrMemberSelfRoleChange", result, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unexpected SQL (a write must not happen): %v", err)
			}
		})
	}
}

func TestUpdateRoleRejectsTargetRoleOutsideTenant(t *testing.T) {
	// The role lookup is tenant-scoped, so a role of another tenant and a missing role
	// both come back as no row.
	repo, mock := newMemberRepositoryWithMock(t)
	f := newRoleUpdateFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND (tenant_id = $2 OR tenant_id IS NULL)`)).
		WithArgs(f.targetRole, f.tenant, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name"}))
	mock.ExpectRollback()

	result, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, otherActor())
	if !errors.Is(err, domain.ErrMemberRoleConflict) || result != nil {
		t.Fatalf("result=%+v err=%v, want ErrMemberRoleConflict", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRoleKeepsCurrentCreatorGuard(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newRoleUpdateFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	f.expectTargetRole(mock, f.tenant, "Staff")
	f.expectCurrentRole(mock, nil, "Creator")
	mock.ExpectRollback()

	result, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, otherActor())
	if !errors.Is(err, domain.ErrMemberRoleForbidden) || result != nil {
		t.Fatalf("result=%+v err=%v, want ErrMemberRoleForbidden", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateRoleTenantRoleNamedCreatorIsNotTheSystemCreator pins that only the system
// Creator role (tenant_id IS NULL) is guarded; a tenant role that happens to be named
// "Creator" carries no special meaning.
func TestUpdateRoleTenantRoleNamedCreatorIsNotTheSystemCreator(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newRoleUpdateFixture()
	mock.ExpectBegin()
	f.expectMember(mock)
	f.expectTargetRole(mock, nil, "Teacher")
	f.expectCurrentRole(mock, f.tenant, "Creator")
	f.expectUpdateAndRead(mock, "Teacher", true)

	if _, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, otherActor()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRoleMissingMemberIsNotFound(t *testing.T) {
	repo, mock := newMemberRepositoryWithMock(t)
	f := newRoleUpdateFixture()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "tenant_members"`)).
		WithArgs(f.member, f.tenant, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	if _, err := repo.UpdateRole(context.Background(), f.tenant, f.member, f.targetRole, otherActor()); !errors.Is(err, domain.ErrMemberNotFound) {
		t.Fatalf("err=%v, want ErrMemberNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
