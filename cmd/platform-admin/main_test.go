package main

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGrantIdempotentAndRecovery(t *testing.T) {
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
	// Both the first grant and recovery after deactivation take the same
	// conflict-safe path. A missing or deleted user cannot be granted.
	for i := 0; i < 2; i++ {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT count").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectExec("INSERT INTO platform_admin_assignments").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("DELETE FROM platform_factor_challenges").WithArgs(id).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if err := grant(context.Background(), db, id); err != nil {
			t.Fatalf("grant %d: %v", i, err)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT count").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	if err := grant(context.Background(), db, id); err == nil {
		t.Fatal("missing user granted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
