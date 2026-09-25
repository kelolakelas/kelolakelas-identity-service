package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConfigurationVersionConflict = errors.New("configuration version conflict")
	ErrConfigurationVersionNotFound = errors.New("configuration version not found")
)

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
