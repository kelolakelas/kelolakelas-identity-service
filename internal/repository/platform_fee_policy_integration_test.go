package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// KEL-99: the fee policy shares the KEL-97 PostgreSQL fixture and the generic
// control-plane tables. Only an applied version is effective; a desired one
// that was never applied yields Applied=false.
func TestPlatformFeePolicyPostgresAppliedAndRollback(t *testing.T) {
	db := kel97DB(t)
	operator := kel97Operator(t, db)
	store := NewConfigurationRepository(db).(*configurationRepository)
	ctx := context.Background()
	state, err := store.PlatformFeeState(ctx)
	if err != nil || !state.Applied || state.PercentBps != 0 || state.FixedFee != 0 || state.DesiredVersion != 0 || state.AppliedVersion != 0 {
		t.Fatalf("baseline: %+v %v", state, err)
	}
	create := func(expected int64, value string) domain.ConfigurationVersion {
		version, err := store.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{Application: domain.PlatformFeeApplication, Environment: domain.PlatformFeeEnvironment, Key: domain.PlatformFeePolicyKey, ExpectedVersion: expected, Value: []byte(value), CreatedBy: operator})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = db.Exec("DELETE FROM configuration_reports WHERE version_id = ?", version.ID).Error
			_ = db.Exec("DELETE FROM configuration_versions WHERE id = ?", version.ID).Error
		})
		return version
	}
	t.Cleanup(func() {
		_ = db.Exec("UPDATE configuration_heads SET latest_version = 0 WHERE application = ? AND environment = ? AND config_key = ?", domain.PlatformFeeApplication, domain.PlatformFeeEnvironment, domain.PlatformFeePolicyKey).Error
	})
	apply := func(version domain.ConfigurationVersion) {
		if _, err := store.RecordConfigurationReport(ctx, domain.ConfigurationReportRequest{VersionID: version.ID, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); err != nil {
			t.Fatal(err)
		}
	}

	first := create(0, `"percent_bps=500,fixed_fee=1000"`)
	state, err = store.PlatformFeeState(ctx)
	if err != nil || state.Applied || state.DesiredVersion != 1 {
		t.Fatalf("desired only: %+v %v", state, err)
	}
	apply(first)
	state, err = store.PlatformFeeState(ctx)
	if err != nil || !state.Applied || state.PercentBps != 500 || state.FixedFee != 1000 || state.AppliedVersion != 1 {
		t.Fatalf("applied: %+v %v", state, err)
	}
	second := create(1, `"percent_bps=2000,fixed_fee=50000"`)
	state, err = store.PlatformFeeState(ctx)
	if err != nil || !state.Applied || state.PercentBps != 500 || state.AppliedVersion != 1 || state.DesiredVersion != 2 {
		t.Fatalf("pending second keeps first: %+v %v", state, err)
	}
	apply(second)
	state, err = store.PlatformFeeState(ctx)
	if err != nil || state.PercentBps != 2000 || state.FixedFee != 50000 || state.AppliedVersion != 2 {
		t.Fatalf("second applied: %+v %v", state, err)
	}
	// Rollback to v1 is a new version (v3) whose applied report wins.
	rollback := create(2, `"percent_bps=500,fixed_fee=1000"`)
	apply(rollback)
	state, err = store.PlatformFeeState(ctx)
	if err != nil || state.PercentBps != 500 || state.FixedFee != 1000 || state.AppliedVersion != 3 {
		t.Fatalf("rollback applied: %+v %v", state, err)
	}
	if _, err := store.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{Application: domain.PlatformFeeApplication, Environment: domain.PlatformFeeEnvironment, Key: domain.PlatformFeePolicyKey, ExpectedVersion: 1, Value: []byte(`"percent_bps=1,fixed_fee=1"`), CreatedBy: operator}); !errors.Is(err, domain.ErrConfigurationVersionConflict) {
		t.Fatalf("stale write: %v", err)
	}
}

func TestPlatformFeePolicyPostgresMissingHeadFailsClosed(t *testing.T) {
	db := kel97DB(t)
	store := NewConfigurationRepository(db).(*configurationRepository)
	ctx := context.Background()
	if err := db.Exec("DELETE FROM configuration_heads WHERE application = ? AND environment = ? AND config_key = ? AND latest_version = 0", domain.PlatformFeeApplication, domain.PlatformFeeEnvironment, domain.PlatformFeePolicyKey).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("INSERT INTO configuration_heads (application, environment, config_key, latest_version) VALUES (?, ?, ?, 0) ON CONFLICT DO NOTHING", domain.PlatformFeeApplication, domain.PlatformFeeEnvironment, domain.PlatformFeePolicyKey).Error
	})
	if _, err := store.PlatformFeeState(ctx); !errors.Is(err, ErrPlatformFeePolicyUnavailable) {
		t.Fatalf("missing head: %v", err)
	}
}
