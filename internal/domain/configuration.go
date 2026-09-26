package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConfigurationVersionConflict = errors.New("configuration version conflict")
	ErrConfigurationVersionNotFound = errors.New("configuration version not found")
)

// Platform registration policy (KEL-97).
//
// TenantOnboardingApplication/TenantRegistrationOpenKey identify the typed
// boolean control-plane setting that decides whether new tenants may register.
// The registration path consults the applied value inside the registration
// transaction, so an applied policy change either commits before the
// registration's policy read or waits until that registration commits.
// RegistrationClosedMessage is the stable, caller-facing domain
// rejection: both the direct handler and the api-gateway pass-through expose
// it unchanged, so clients can branch on it.
const (
	TenantOnboardingApplication   = "identity"
	TenantRegistrationOpenKey     = "TENANT_REGISTRATION_OPEN"
	TenantRegistrationEnvironment = "platform"
	RegistrationClosedMessage     = "New tenant registration is currently closed"
)

var ErrRegistrationClosed = errors.New(RegistrationClosedMessage)

// RegistrationOpenFromApplied decodes the applied boolean value of the
// registration setting. An absent setting, a missing applied acknowledgement,
// a stored null, or a stored non-boolean scalar all fail closed: new-tenant
// registration keeps working only while the control plane positively reports
// the applied value as true.
func RegistrationOpenFromApplied(state *ConfigurationState) (bool, error) {
	if state == nil || state.LastKnownGood == nil {
		return false, errors.New("registration policy has no applied configuration version")
	}
	trimmed := bytes.TrimSpace(state.LastKnownGood.Value)
	if string(trimmed) != "true" {
		if string(trimmed) == "false" {
			return false, nil
		}
		return false, fmt.Errorf("registration policy applied value must be a boolean, got %q", string(trimmed))
	}
	return true, nil
}

// RegistrationPolicyEvaluated is the audit view of one effective registration
// policy read. It reports the applied and desired versions that decided the
// outcome so operators can correlate a rejection with the control plane state.
type RegistrationPolicyEvaluated struct {
	Application    string `json:"application"`
	Key            string `json:"key"`
	Environment    string `json:"environment"`
	Open           bool   `json:"open"`
	AppliedVersion int64  `json:"applied_version"`
	DesiredVersion int64  `json:"desired_version"`
}

// Platform public catalog visibility policy (KEL-98).
//
// PublicCatalogApplication/PublicCatalogOpenKey identify the typed boolean
// control-plane setting, owned by academic, that decides whether the public
// class catalog (list and detail) shows any class. Like the registration policy
// the effective value is the applied version: a desired version only takes
// effect once it is acknowledged as applied, and version 0 (the seeded head
// with no operator version) is the default-open behaviour. The setting never
// changes class or tenant rows; academic hides catalog output while it is false.
const (
	PublicCatalogApplication = "academic"
	PublicCatalogOpenKey     = "PUBLIC_CATALOG_OPEN"
	PublicCatalogEnvironment = "platform"
)

// PublicCatalogPolicyEvaluated is the audit view of one effective public
// catalog policy read: the outcome plus the applied and desired versions that
// decided it. Academic receives the same view over gRPC, so the version it
// enforces is the version platform admins see.
type PublicCatalogPolicyEvaluated struct {
	Application    string `json:"application"`
	Key            string `json:"key"`
	Environment    string `json:"environment"`
	Open           bool   `json:"open"`
	AppliedVersion int64  `json:"applied_version"`
	DesiredVersion int64  `json:"desired_version"`
}

// PublicCatalogPolicy gates the public class catalog (KEL-98). Evaluate never
// returns (Open=true, nil) unless the control plane positively reports an open
// applied value (or the seeded default): a missing head, an unreadable store,
// or a non-boolean applied value return an error so callers fail closed. Close
// and Open append a desired version exactly like any other configuration value.
type PublicCatalogPolicy interface {
	Evaluate(context.Context) (PublicCatalogPolicyEvaluated, error)
	Close(context.Context, uuid.UUID) (PublicCatalogPolicyEvaluated, error)
	Open(context.Context, uuid.UUID) (PublicCatalogPolicyEvaluated, error)
}

type ConfigurationStatus string

const (
	ConfigurationStatusNotConfigured ConfigurationStatus = "not_configured"
	ConfigurationStatusRequested     ConfigurationStatus = "requested"
	ConfigurationStatusApplied       ConfigurationStatus = "applied"
	ConfigurationStatusFailed        ConfigurationStatus = "failed"
	ConfigurationStatusRollback      ConfigurationStatus = "rollback"
)

type ConfigurationDefinition struct {
	Application       string   `json:"application"`
	Owner             string   `json:"owner"`
	Key               string   `json:"key"`
	Type              string   `json:"type"`
	Required          bool     `json:"required"`
	Default           string   `json:"default"`
	Description       string   `json:"description"`
	Sensitive         bool     `json:"sensitive"`
	Validation        string   `json:"validation"`
	ApplicationMethod string   `json:"application_method"`
	BootstrapOnly     bool     `json:"bootstrap_only"`
	Allowed           []string `json:"allowed_values,omitempty"`
}

type ConfigurationVersion struct {
	ID                  int64               `json:"id" gorm:"column:id"`
	Application         string              `json:"application" gorm:"column:application"`
	Environment         string              `json:"environment" gorm:"column:environment"`
	Key                 string              `json:"key" gorm:"column:config_key"`
	Version             int64               `json:"version" gorm:"column:version"`
	Value               json.RawMessage     `json:"value,omitempty" gorm:"column:value"`
	CreatedBy           uuid.UUID           `json:"created_by" gorm:"column:created_by"`
	CreatedAt           time.Time           `json:"created_at" gorm:"column:created_at"`
	RollbackOfVersionID *int64              `json:"rollback_of_version_id,omitempty" gorm:"column:rollback_of_version_id"`
	Status              ConfigurationStatus `json:"status,omitempty" gorm:"-"`
}

type ConfigurationReport struct {
	ID         int64               `json:"id" gorm:"column:id"`
	VersionID  int64               `json:"version_id" gorm:"column:version_id"`
	Status     ConfigurationStatus `json:"status" gorm:"column:status"`
	ReportedBy uuid.UUID           `json:"reported_by" gorm:"column:reported_by"`
	CreatedAt  time.Time           `json:"created_at" gorm:"column:created_at"`
}

type ConfigurationState struct {
	Latest        *ConfigurationVersion
	LastKnownGood *ConfigurationVersion
}

type ConfigurationVersionRequest struct {
	Application         string
	Environment         string
	Key                 string
	ExpectedVersion     int64
	Value               json.RawMessage
	RollbackOfVersionID *int64
	CreatedBy           uuid.UUID
}

type ConfigurationReportRequest struct {
	VersionID  int64
	Status     ConfigurationStatus
	ReportedBy uuid.UUID
}

type ConfigurationHistory struct {
	Versions []ConfigurationVersion `json:"versions"`
	Reports  []ConfigurationReport  `json:"reports"`
}

type ConfigurationRepository interface {
	ListConfigurationState(context.Context, string) (map[string]ConfigurationState, error)
	GetConfigurationHistory(context.Context, string, string, string) (ConfigurationHistory, error)
	CreateConfigurationVersion(context.Context, ConfigurationVersionRequest) (ConfigurationVersion, error)
	RecordConfigurationReport(context.Context, ConfigurationReportRequest) (ConfigurationReport, error)
}

// RegistrationPolicy gates new-tenant registration (KEL-97). Evaluate reads
// the effective policy and never returns (true, nil) unless the control plane
// positively reports an applied, open value: an unavailable store, an absent
// setting, or an undesired unacknowledged change all fail closed. Close and
// Open append a new desired version so the policy stays versioned and audited
// exactly like every other configuration value.
type RegistrationPolicy interface {
	Evaluate(context.Context) (RegistrationPolicyEvaluated, error)
	Close(context.Context, uuid.UUID) (RegistrationPolicyEvaluated, error)
	Open(context.Context, uuid.UUID) (RegistrationPolicyEvaluated, error)
}
