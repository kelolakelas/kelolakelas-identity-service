package repository

import (
	"context"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"testing"
)

// Uses the isolated, migrated PostgreSQL fixture from KEL-97. The policy
// version and report share the generic control-plane tables with registration.
func TestPublicCatalogPolicyPostgresAppliedAndRollback(t *testing.T) {
	db := kel97DB(t)
	operator := kel97Operator(t, db)
	store := NewConfigurationRepository(db)
	ctx := context.Background()
	state, err := store.(*configurationRepository).PublicCatalogState(ctx)
	if err != nil || !state.Open || state.DesiredVersion != 0 || state.AppliedVersion != 0 {
		t.Fatalf("default: %+v %v", state, err)
	}
	close, err := store.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{Application: domain.PublicCatalogApplication, Environment: domain.PublicCatalogEnvironment, Key: domain.PublicCatalogOpenKey, ExpectedVersion: 0, Value: []byte("false"), CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM configuration_reports WHERE version_id = ?", close.ID).Error
		_ = db.Exec("DELETE FROM configuration_versions WHERE id = ?", close.ID).Error
		_ = db.Exec("UPDATE configuration_heads SET latest_version = 0 WHERE application = ? AND environment = ? AND config_key = ?", domain.PublicCatalogApplication, domain.PublicCatalogEnvironment, domain.PublicCatalogOpenKey).Error
	})
	state, err = store.(*configurationRepository).PublicCatalogState(ctx)
	if err != nil || state.Open || state.DesiredVersion != 1 || state.AppliedVersion != 0 {
		t.Fatalf("unapplied: %+v %v", state, err)
	}
	if _, err = store.RecordConfigurationReport(ctx, domain.ConfigurationReportRequest{VersionID: close.ID, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); err != nil {
		t.Fatal(err)
	}
	state, err = store.(*configurationRepository).PublicCatalogState(ctx)
	if err != nil || state.Open || state.AppliedVersion != 1 {
		t.Fatalf("closed: %+v %v", state, err)
	}
	reopen, err := store.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{Application: domain.PublicCatalogApplication, Environment: domain.PublicCatalogEnvironment, Key: domain.PublicCatalogOpenKey, ExpectedVersion: 1, Value: []byte("true"), CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM configuration_reports WHERE version_id = ?", reopen.ID).Error
		_ = db.Exec("DELETE FROM configuration_versions WHERE id = ?", reopen.ID).Error
	})
	state, err = store.(*configurationRepository).PublicCatalogState(ctx)
	if err != nil || state.Open || state.AppliedVersion != 1 || state.DesiredVersion != 2 {
		t.Fatalf("pending reopen: %+v %v", state, err)
	}
	if _, err = store.RecordConfigurationReport(ctx, domain.ConfigurationReportRequest{VersionID: reopen.ID, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); err != nil {
		t.Fatal(err)
	}
	state, err = store.(*configurationRepository).PublicCatalogState(ctx)
	if err != nil || !state.Open || state.AppliedVersion != 2 {
		t.Fatalf("reopened: %+v %v", state, err)
	}
}
