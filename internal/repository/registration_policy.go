package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

// registrationOpenInTx locks the singleton head until the registration commits.
// The policy report writer locks the same row before inserting an applied report.
func registrationOpenInTx(tx *gorm.DB) (bool, error) {
	var version int64
	err := tx.Raw(`SELECT latest_version FROM configuration_heads WHERE application = ? AND environment = ? AND config_key = ? FOR SHARE`, domain.TenantOnboardingApplication, domain.TenantRegistrationEnvironment, domain.TenantRegistrationOpenKey).Row().Scan(&version)
	if err != nil {
		return false, err
	}
	if version == 0 {
		return true, nil
	}
	var value []byte
	err = tx.Raw(`SELECT v.value FROM configuration_versions v JOIN configuration_reports r ON r.version_id = v.id WHERE v.application = ? AND v.environment = ? AND v.config_key = ? AND r.status = 'applied' ORDER BY r.id DESC LIMIT 1`, domain.TenantOnboardingApplication, domain.TenantRegistrationEnvironment, domain.TenantRegistrationOpenKey).Row().Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if string(value) != "true" && string(value) != "false" {
		return false, errors.New("invalid registration policy")
	}
	return string(value) == "true", nil
}

// RegistrationState returns the effective applied policy, failing closed if
// the store or its first acknowledged value is unavailable.
func (r *configurationRepository) RegistrationState(ctx context.Context) (domain.RegistrationPolicyEvaluated, error) {
	result := domain.RegistrationPolicyEvaluated{Application: domain.TenantOnboardingApplication, Environment: domain.TenantRegistrationEnvironment, Key: domain.TenantRegistrationOpenKey}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var desired int64
		err := tx.Raw(`SELECT latest_version FROM configuration_heads WHERE application = ? AND environment = ? AND config_key = ?`, result.Application, result.Environment, result.Key).Row().Scan(&desired)
		if err != nil {
			return err
		}
		result.DesiredVersion = desired
		if desired == 0 {
			result.Open = true
			return nil
		}
		var value []byte
		err = tx.Raw(`SELECT v.version, v.value FROM configuration_versions v JOIN configuration_reports r ON r.version_id = v.id WHERE v.application = ? AND v.environment = ? AND v.config_key = ? AND r.status = 'applied' ORDER BY r.id DESC LIMIT 1`, result.Application, result.Environment, result.Key).Row().Scan(&result.AppliedVersion, &value)
		if errors.Is(err, sql.ErrNoRows) {
			result.Open = true
			return nil
		}
		if err != nil {
			return err
		}
		if string(value) != "true" && string(value) != "false" {
			return errors.New("invalid registration policy")
		}
		result.Open = string(value) == "true"
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return result, errors.New("registration policy unavailable")
	}
	return result, err
}
