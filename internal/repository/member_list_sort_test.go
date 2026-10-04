package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// newCapturingMemberListRepository records every SQL statement the List query
// generates. The count query is answered with total, and the row query with an
// empty member set, so List returns without reaching role_permissions.
func newCapturingMemberListRepository(t *testing.T, total int64) (*memberRepository, *[]string) {
	t.Helper()
	captured := new([]string)
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(
		func(_, actualSQL string) error {
			*captured = append(*captured, actualSQL)
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
	mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	mock.ExpectQuery(".*").WillReturnRows(sqlmock.NewRows([]string{
		"id", "user_id", "tenant_id", "email", "first_name", "last_name", "phone",
		"status", "role_id", "role_name", "is_system_role", "created_at", "updated_at",
	}))
	return &memberRepository{db: db}, captured
}

func memberListSQL(t *testing.T, query domain.MemberQuery) string {
	t.Helper()
	repo, captured := newCapturingMemberListRepository(t, 0)
	if _, _, err := repo.List(context.Background(), uuid.New(), domain.MemberQuery{
		Page: 1, PageSize: 20, Sort: query.Sort, Order: query.Order,
	}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(*captured) < 2 {
		t.Fatalf("expected count and row queries, got %d", len(*captured))
	}
	return (*captured)[1]
}

// TestMemberListOrderClause pins the KEL-165 repository contract: every
// supported sort (including the created_at alias and the empty sort the handler
// normalizes) orders by a real column with tm.id as the secondary key, and the
// old tm.created_at fallback column never appears.
func TestMemberListOrderClause(t *testing.T) {
	tests := []struct {
		name      string
		sort      string
		order     string
		wantOrder string
	}{
		{name: "empty sort orders by joined_at asc", sort: "", wantOrder: "tm.joined_at ASC, tm.id ASC"},
		{name: "joined_at orders by joined_at asc", sort: "joined_at", wantOrder: "tm.joined_at ASC, tm.id ASC"},
		{name: "created_at alias orders by joined_at asc", sort: "created_at", wantOrder: "tm.joined_at ASC, tm.id ASC"},
		{name: "updated_at orders by updated_at asc", sort: "updated_at", wantOrder: "tm.updated_at ASC, tm.id ASC"},
		{name: "email orders by user email asc", sort: "email", wantOrder: "u.email ASC, tm.id ASC"},
		{name: "name orders by last name asc", sort: "name", wantOrder: "u.last_name ASC, tm.id ASC"},
		{name: "desc order without sort keeps joined_at desc", sort: "", order: "desc", wantOrder: "tm.joined_at DESC, tm.id DESC"},
		{name: "desc order applies to every sort", sort: "email", order: "desc", wantOrder: "u.email DESC, tm.id DESC"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sql := memberListSQL(t, domain.MemberQuery{Sort: test.sort, Order: test.order})
			if !strings.Contains(sql, "ORDER BY "+test.wantOrder) {
				t.Fatalf("SQL is missing ORDER BY %q: %s", test.wantOrder, sql)
			}
			if strings.Contains(sql, "tm.created_at") {
				t.Fatalf("SQL references the removed tm.created_at column: %s", sql)
			}
		})
	}
}
