package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

// ErrPlatformFeePolicyUnavailable reports that the seeded policy head is
// missing, so no applied fee rule can be trusted. Billing fails closed.
var ErrPlatformFeePolicyUnavailable = errors.New("platform fee policy unavailable")

// PlatformFeeState returns the effective applied platform fee policy (KEL-99).
// The head seeded by migration 000013 at version 0 is the explicit applied
// baseline of 0 bps + Rp0. Once an operator version exists, the most recent
// applied report decides (a rollback is itself a new version that must be
// applied). A desired version with no applied report at all yields
// Applied=false: the rule is unknown and must not be enforced. A missing head,
// a read error, or a malformed stored value return an error; there is no
// default.
func (r *configurationRepository) PlatformFeeState(ctx context.Context) (domain.PlatformFeePolicyEvaluated, error) {
	result := domain.PlatformFeePolicyEvaluated{Application: domain.PlatformFeeApplication, Environment: domain.PlatformFeeEnvironment, Key: domain.PlatformFeePolicyKey}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var desired int64
		err := tx.Raw(`SELECT latest_version FROM configuration_heads WHERE application = ? AND environment = ? AND config_key = ?`, result.Application, result.Environment, result.Key).Row().Scan(&desired)
		if err != nil {
			return err
		}
		result.DesiredVersion = desired
		if desired == 0 {
			result.Applied = true
			return nil
		}
		var raw []byte
		err = tx.Raw(`SELECT v.version, v.value FROM configuration_versions v JOIN configuration_reports r ON r.version_id = v.id WHERE v.application = ? AND v.environment = ? AND v.config_key = ? AND r.status = 'applied' ORDER BY r.id DESC LIMIT 1`, result.Application, result.Environment, result.Key).Row().Scan(&result.AppliedVersion, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			// The baseline stopped being the recorded state when an operator
			// wrote a version; until that version is applied nothing is.
			return nil
		}
		if err != nil {
			return err
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return domain.ErrInvalidPlatformFeePolicy
		}
		value, err := domain.ParsePlatformFeePolicyValue(text)
		if err != nil {
			return err
		}
		result.Applied = true
		result.PercentBps, result.FixedFee = value.PercentBps, value.FixedFee
		return nil
	})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PlatformFeePolicyEvaluated{}, ErrPlatformFeePolicyUnavailable
	}
	if err != nil {
		return domain.PlatformFeePolicyEvaluated{}, err
	}
	return result, nil
}
