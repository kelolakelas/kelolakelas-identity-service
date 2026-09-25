package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"gorm.io/gorm"
)

type configurationRepository struct{ db *gorm.DB }

func NewConfigurationRepository(db *gorm.DB) domain.ConfigurationRepository {
	return &configurationRepository{db: db}
}

func (r *configurationRepository) ListConfigurationState(ctx context.Context, environment string) (map[string]domain.ConfigurationState, error) {
	const query = `
SELECT latest.application, latest.config_key, latest.id, latest.version, latest.value,
       latest.created_by, latest.created_at, COALESCE(current_report.status, 'requested'),
       good.id, good.version, good.created_by, good.created_at
FROM (
    SELECT DISTINCT ON (application, config_key) *
    FROM configuration_versions
    WHERE environment = ?
    ORDER BY application, config_key, version DESC
) AS latest
LEFT JOIN LATERAL (
    SELECT status FROM configuration_reports
    WHERE version_id = latest.id
    ORDER BY id DESC LIMIT 1
) AS current_report ON true
LEFT JOIN LATERAL (
    SELECT v.id, v.version, v.created_by, v.created_at
    FROM configuration_versions v
    JOIN configuration_reports report ON report.version_id = v.id
    WHERE v.application = latest.application
      AND v.environment = latest.environment
      AND v.config_key = latest.config_key
      AND report.status = 'applied'
    ORDER BY report.id DESC LIMIT 1
) AS good ON true`

	rows, err := r.db.WithContext(ctx).Raw(query, environment).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[string]domain.ConfigurationState)
	for rows.Next() {
		var application, key, status string
		var latestID, latestVersion int64
		var latestValue []byte
		var latestCreatedBy uuid.UUID
		var latestCreatedAt sql.NullTime
		var goodID, goodVersion sql.NullInt64
		var goodCreatedBy uuid.NullUUID
		var goodCreatedAt sql.NullTime
		if err := rows.Scan(&application, &key, &latestID, &latestVersion, &latestValue, &latestCreatedBy, &latestCreatedAt, &status, &goodID, &goodVersion, &goodCreatedBy, &goodCreatedAt); err != nil {
			return nil, err
		}
		latest := &domain.ConfigurationVersion{ID: latestID, Application: application, Environment: environment, Key: key, Version: latestVersion, Value: latestValue, CreatedBy: latestCreatedBy, Status: domain.ConfigurationStatus(status)}
		if latestCreatedAt.Valid {
			latest.CreatedAt = latestCreatedAt.Time
		}
		state := domain.ConfigurationState{Latest: latest}
		if goodID.Valid {
			lastKnownGood := &domain.ConfigurationVersion{ID: goodID.Int64, Application: application, Environment: environment, Key: key, Version: goodVersion.Int64}
			if goodCreatedBy.Valid {
				lastKnownGood.CreatedBy = goodCreatedBy.UUID
			}
			if goodCreatedAt.Valid {
				lastKnownGood.CreatedAt = goodCreatedAt.Time
			}
			state.LastKnownGood = lastKnownGood
		}
		states[application+"\x00"+key] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return states, nil
}

func (r *configurationRepository) GetConfigurationHistory(ctx context.Context, application, environment, key string) (domain.ConfigurationHistory, error) {
	var history domain.ConfigurationHistory
	if err := r.db.WithContext(ctx).Table("configuration_versions").
		Where("application = ? AND environment = ? AND config_key = ?", application, environment, key).
		Order("version DESC").Find(&history.Versions).Error; err != nil {
		return domain.ConfigurationHistory{}, err
	}
	if len(history.Versions) == 0 {
		return history, nil
	}
	ids := make([]int64, 0, len(history.Versions))
	for _, version := range history.Versions {
		ids = append(ids, version.ID)
	}
	if err := r.db.WithContext(ctx).Table("configuration_reports").
		Where("version_id IN ?", ids).Order("id DESC").Find(&history.Reports).Error; err != nil {
		return domain.ConfigurationHistory{}, err
	}
	return history, nil
}

func (r *configurationRepository) CreateConfigurationVersion(ctx context.Context, request domain.ConfigurationVersionRequest) (domain.ConfigurationVersion, error) {
	var created domain.ConfigurationVersion
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var version int64
		const advance = `
INSERT INTO configuration_heads (application, environment, config_key, latest_version)
SELECT ?, ?, ?, 1
WHERE ? = 0 OR EXISTS (
    SELECT 1 FROM configuration_heads
    WHERE application = ? AND environment = ? AND config_key = ?
)
ON CONFLICT (application, environment, config_key) DO UPDATE
SET latest_version = configuration_heads.latest_version + 1, updated_at = now()
WHERE configuration_heads.latest_version = ?
RETURNING latest_version`
		err := tx.Raw(advance, request.Application, request.Environment, request.Key, request.ExpectedVersion, request.Application, request.Environment, request.Key, request.ExpectedVersion).Row().Scan(&version)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrConfigurationVersionConflict
		}
		if err != nil {
			return err
		}
		const insert = `
INSERT INTO configuration_versions
    (application, environment, config_key, version, value, created_by, rollback_of_version_id)
VALUES (?, ?, ?, ?, ?::jsonb, ?, ?)
RETURNING id, application, environment, config_key, version, value, created_by, created_at, rollback_of_version_id`
		row := tx.Raw(insert, request.Application, request.Environment, request.Key, version, string(request.Value), request.CreatedBy, request.RollbackOfVersionID).Row()
		return row.Scan(&created.ID, &created.Application, &created.Environment, &created.Key, &created.Version, &created.Value, &created.CreatedBy, &created.CreatedAt, &created.RollbackOfVersionID)
	})
	if err != nil {
		return domain.ConfigurationVersion{}, err
	}
	created.Status = domain.ConfigurationStatusRequested
	return created, nil
}

func (r *configurationRepository) RecordConfigurationReport(ctx context.Context, request domain.ConfigurationReportRequest) (domain.ConfigurationReport, error) {
	var report domain.ConfigurationReport
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize an applied registration-policy acknowledgement against
		// registrations holding FOR SHARE on this head. Resolve the version
		// before locking so unrelated configuration reports remain independent.
		var application, environment, key string
		if err := tx.Raw(`SELECT application, environment, config_key FROM configuration_versions WHERE id = ?`, request.VersionID).Row().Scan(&application, &environment, &key); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrConfigurationVersionNotFound
			}
			return err
		}
		if application == domain.TenantOnboardingApplication && environment == domain.TenantRegistrationEnvironment && key == domain.TenantRegistrationOpenKey && request.Status == domain.ConfigurationStatusApplied {
			var head int64
			if err := tx.Raw(`SELECT latest_version FROM configuration_heads WHERE application = ? AND environment = ? AND config_key = ? FOR UPDATE`, application, environment, key).Row().Scan(&head); err != nil {
				return err
			}
		}
		const insert = `
INSERT INTO configuration_reports (version_id, status, reported_by)
SELECT id, ?, ? FROM configuration_versions WHERE id = ?
RETURNING id, version_id, status, reported_by, created_at`
		if err := tx.Raw(insert, request.Status, request.ReportedBy, request.VersionID).Row().Scan(&report.ID, &report.VersionID, &report.Status, &report.ReportedBy, &report.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrConfigurationVersionNotFound
			}
			return err
		}
		return nil
	})
	if err != nil {
		return domain.ConfigurationReport{}, err
	}
	return report, nil
}
