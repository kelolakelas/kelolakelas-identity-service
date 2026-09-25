package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newConfigurationRepositoryWithMock(t *testing.T) (*configurationRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &configurationRepository{db: db}, mock
}

func TestCreateConfigurationVersionAdvancesHeadAndInsertsAtomically(t *testing.T) {
	repository, mock := newConfigurationRepositoryWithMock(t)
	operator := uuid.New()
	createdAt := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO configuration_heads")).
		WithArgs("api-gateway", "staging", "RATE_LIMIT_REQUESTS", int64(0), "api-gateway", "staging", "RATE_LIMIT_REQUESTS", int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"latest_version"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO configuration_versions")).
		WithArgs("api-gateway", "staging", "RATE_LIMIT_REQUESTS", int64(1), "75", sqlmock.AnyArg(), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "application", "environment", "config_key", "version", "value", "created_by", "created_at", "rollback_of_version_id"}).
			AddRow(int64(4), "api-gateway", "staging", "RATE_LIMIT_REQUESTS", int64(1), []byte("75"), operator.String(), createdAt, nil))
	mock.ExpectCommit()

	version, err := repository.CreateConfigurationVersion(context.Background(), domain.ConfigurationVersionRequest{
		Application: "api-gateway", Environment: "staging", Key: "RATE_LIMIT_REQUESTS",
		ExpectedVersion: 0, Value: []byte("75"), CreatedBy: operator,
	})
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	if version.ID != 4 || version.Version != 1 || string(version.Value) != "75" || version.Status != domain.ConfigurationStatusRequested {
		t.Fatalf("version=%+v", version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCreateConfigurationVersionConflictRollsBackHeadAdvance(t *testing.T) {
	repository, mock := newConfigurationRepositoryWithMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO configuration_heads")).
		WithArgs("api-gateway", "prod", "RATE_LIMIT_REQUESTS", int64(1), "api-gateway", "prod", "RATE_LIMIT_REQUESTS", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"latest_version"}))
	mock.ExpectRollback()

	_, err := repository.CreateConfigurationVersion(context.Background(), domain.ConfigurationVersionRequest{
		Application: "api-gateway", Environment: "prod", Key: "RATE_LIMIT_REQUESTS",
		ExpectedVersion: 1, Value: []byte("90"), CreatedBy: uuid.New(),
	})
	if err != domain.ErrConfigurationVersionConflict {
		t.Fatalf("error=%v want=%v", err, domain.ErrConfigurationVersionConflict)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestListConfigurationStateKeepsLatestFailureAndAppliedVersion(t *testing.T) {
	repository, mock := newConfigurationRepositoryWithMock(t)
	latestCreatedAt := time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC)
	goodCreatedAt := latestCreatedAt.Add(-time.Hour)
	operator := uuid.New()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT latest.application")).
		WithArgs("prod").
		WillReturnRows(sqlmock.NewRows([]string{
			"application", "config_key", "id", "version", "value", "created_by", "created_at", "status",
			"good_id", "good_version", "good_created_by", "good_created_at",
		}).AddRow("api-gateway", "RATE_LIMIT_REQUESTS", int64(5), int64(2), []byte("90"), operator.String(), latestCreatedAt,
			string(domain.ConfigurationStatusFailed), int64(4), int64(1), operator.String(), goodCreatedAt))

	states, err := repository.ListConfigurationState(context.Background(), "prod")
	if err != nil {
		t.Fatalf("list state: %v", err)
	}
	state, ok := states["api-gateway\x00RATE_LIMIT_REQUESTS"]
	if !ok || state.Latest == nil || state.LastKnownGood == nil {
		t.Fatalf("state missing latest or last-known-good: %+v", states)
	}
	if state.Latest.Version != 2 || string(state.Latest.Value) != "90" || state.Latest.Status != domain.ConfigurationStatusFailed {
		t.Fatalf("latest=%+v", state.Latest)
	}
	if state.LastKnownGood.Version != 1 || state.LastKnownGood.ID != 4 {
		t.Fatalf("last known good=%+v", state.LastKnownGood)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRecordConfigurationReportDoesNotSucceedForUnknownVersion(t *testing.T) {
	repository, mock := newConfigurationRepositoryWithMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT application, environment, config_key FROM configuration_versions WHERE id = $1")).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"application", "environment", "config_key"}))
	mock.ExpectRollback()

	_, err := repository.RecordConfigurationReport(context.Background(), domain.ConfigurationReportRequest{
		VersionID: 999, Status: domain.ConfigurationStatusApplied, ReportedBy: uuid.New(),
	})
	if err != domain.ErrConfigurationVersionNotFound {
		t.Fatalf("error=%v want=%v", err, domain.ErrConfigurationVersionNotFound)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
