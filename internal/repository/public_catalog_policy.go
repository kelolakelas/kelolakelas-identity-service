package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

// ErrPublicCatalogPolicyUnavailable reports that the seeded policy head is
// missing, so the effective value cannot be trusted. Callers fail closed.
var ErrPublicCatalogPolicyUnavailable = errors.New("public catalog policy unavailable")

// PublicCatalogState returns the effective applied public catalog policy
// (KEL-98). The head seeded by migration 000009 at version 0 is the default
// open policy; once an operator version exists, the most recent applied report
// decides. A desired version without any applied report is untrusted and
// fails closed until the first acknowledgement arrives.
// A missing head, a read error, or a stored non-boolean value return an error.
func (r *configurationRepository) PublicCatalogState(ctx context.Context) (domain.PublicCatalogPolicyEvaluated, error) {
	result := domain.PublicCatalogPolicyEvaluated{Application: domain.PublicCatalogApplication, Environment: domain.PublicCatalogEnvironment, Key: domain.PublicCatalogOpenKey}
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
			// A desired-only version has no trusted applied value. Hide
			// the catalog until its first applied report arrives.
			result.Open = false
			return nil
		}
		if err != nil {
			return err
		}
		switch string(value) {
		case "true":
			result.Open = true
		case "false":
			result.Open = false
		default:
			return errors.New("invalid public catalog policy value")
		}
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublicCatalogPolicyEvaluated{}, ErrPublicCatalogPolicyUnavailable
	}
	if err != nil {
		return domain.PublicCatalogPolicyEvaluated{}, err
	}
	return result, nil
}
