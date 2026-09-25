package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestIsNameExistsExceptExcludesCurrentTenantAndPropagatesQueryError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int64
		err   error
		want  bool
	}{
		{name: "other tenant", count: 1, want: true},
		{name: "own name", count: 0},
		{name: "database failure", err: errors.New("unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			id := uuid.New()
			expected := mock.ExpectQuery(regexp.QuoteMeta("LOWER(name) = LOWER($1) AND id <> $2")).WithArgs("CURRENT TENANT", id)
			if tc.err != nil {
				expected.WillReturnError(tc.err)
			} else {
				expected.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.count))
			}
			got, err := (&tenantRepository{db: db}).IsNameExistsExcept(context.Background(), "CURRENT TENANT", id)
			if !errors.Is(err, tc.err) || got != tc.want {
				t.Fatalf("exists=%v, err=%v; want %v, %v", got, err, tc.want, tc.err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
