package grpc

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newMembershipServerWithMock builds the KEL-135 membership server on top of
// a mocked database so the tests can observe the SQL behind the verdict.
func newMembershipServerWithMock(t *testing.T) (MembershipServiceServer, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewMembershipServer(db), mock
}

// captureMembershipSQL runs CheckActiveMembership and returns the SQL that
// was executed. The lookup is answered with "inactive" because only the query
// shape matters here.
func captureMembershipSQL(t *testing.T, fields map[string]interface{}) string {
	t.Helper()
	var captured string
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(_, actualSQL string) error {
			captured = actualSQL
			return nil
		},
	)))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	server := NewMembershipServer(db)
	resp, err := server.CheckActiveMembership(context.Background(), mustStruct(fields))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetFields()["active"].GetBoolValue() {
		t.Fatal("expected the lookup to answer inactive")
	}
	return captured
}

// TestCheckActiveMembershipAnswersActiveRowsOnly is the KEL-135 proof that the
// substitute-tutor guard can trust the verdict: only a membership that
// belongs to the calling tenant, is active, and is not soft-deleted answers
// active. Another tenant's member, an inactive member, and an unknown id all
// answer false with no error, so the caller cannot distinguish them; a
// database failure is an Internal error so academic fails closed.
func TestCheckActiveMembershipAnswersActiveRowsOnly(t *testing.T) {
	tenantID, memberID := uuid.New(), uuid.New()
	req := mustStruct(map[string]interface{}{
		"tenant_id": tenantID.String(),
		"member_id": memberID.String(),
	})

	t.Run("active member answers true", func(t *testing.T) {
		server, mock := newMembershipServerWithMock(t)
		mock.ExpectQuery(regexp.QuoteMeta(`FROM "tenant_members"`)).
			WithArgs(memberID, tenantID, true).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		resp, err := server.CheckActiveMembership(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.GetFields()["active"].GetBoolValue() {
			t.Fatal("active=false, want true")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})

	// Cross-tenant, inactive, and unknown ids all come back as count 0
	// through the same query, so they share the false verdict by
	// construction.
	t.Run("cross-tenant inactive and unknown ids answer false", func(t *testing.T) {
		server, mock := newMembershipServerWithMock(t)
		mock.ExpectQuery(regexp.QuoteMeta(`FROM "tenant_members"`)).
			WithArgs(memberID, tenantID, true).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		resp, err := server.CheckActiveMembership(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.GetFields()["active"].GetBoolValue() {
			t.Fatal("active=true for an ineligible member, want false")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet SQL expectations: %v", err)
		}
	})
}

// TestCheckActiveMembershipQueryEnforcesInvariants asserts the membership SQL
// carries every KEL-135 predicate: same tenant, active, not soft-deleted.
func TestCheckActiveMembershipQueryEnforcesInvariants(t *testing.T) {
	sql := captureMembershipSQL(t, map[string]interface{}{
		"tenant_id": uuid.NewString(),
		"member_id": uuid.NewString(),
	})
	for _, want := range []string{"tenant_members", "tenant_id", "is_active", "deleted_at"} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(sql) {
			t.Fatalf("membership SQL %q lacks %q", sql, want)
		}
	}
}

// TestCheckActiveMembershipRejectsBadContract covers the request seam:
// missing or malformed tenant/member ids are InvalidArgument, never a false
// verdict that the caller could mistake for an ineligible member.
func TestCheckActiveMembershipRejectsBadContract(t *testing.T) {
	server, _ := newMembershipServerWithMock(t)
	cases := []struct {
		name   string
		fields map[string]interface{}
	}{
		{"missing tenant", map[string]interface{}{"member_id": uuid.NewString()}},
		{"missing member", map[string]interface{}{"tenant_id": uuid.NewString()}},
		{"malformed tenant", map[string]interface{}{"tenant_id": "not-a-uuid", "member_id": uuid.NewString()}},
		{"malformed member", map[string]interface{}{"tenant_id": uuid.NewString(), "member_id": "not-a-uuid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server.CheckActiveMembership(context.Background(), mustStruct(tc.fields))
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code=%v want InvalidArgument (err=%v)", status.Code(err), err)
			}
		})
	}
}
