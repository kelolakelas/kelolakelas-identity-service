package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type ConfigurationValidationError struct{ message string }

func (e ConfigurationValidationError) Error() string { return e.message }

func invalidConfiguration(message string) error {
	return ConfigurationValidationError{message: message}
}

func IsConfigurationValidationError(err error) bool {
	var validationError ConfigurationValidationError
	return errors.As(err, &validationError)
}

type ConfigurationControlPlane struct {
	repository domain.ConfigurationRepository
}

func NewConfigurationControlPlane(repository domain.ConfigurationRepository) *ConfigurationControlPlane {
	return &ConfigurationControlPlane{repository: repository}
}

type ConfigurationInventoryItem struct {
	Definition           domain.ConfigurationDefinition `json:"definition"`
	Environment          string                         `json:"environment"`
	Status               domain.ConfigurationStatus     `json:"status"`
	LatestVersion        int64                          `json:"latest_version"`
	LatestVersionStatus  domain.ConfigurationStatus     `json:"latest_version_status,omitempty"`
	Value                json.RawMessage                `json:"value,omitempty"`
	LastKnownGoodVersion int64                          `json:"last_known_good_version,omitempty"`
}

type ConfigurationInventory struct {
	Environment    string                       `json:"environment"`
	Configurations []ConfigurationInventoryItem `json:"configurations"`
}

func (c *ConfigurationControlPlane) Inventory(ctx context.Context, environment string) (ConfigurationInventory, error) {
	environment, err := normalizeEnvironment(environment)
	if err != nil {
		return ConfigurationInventory{}, err
	}
	states, err := c.repository.ListConfigurationState(ctx, environment)
	if err != nil {
		return ConfigurationInventory{}, err
	}
	catalog := ConfigurationCatalog()
	inventory := ConfigurationInventory{Environment: environment, Configurations: make([]ConfigurationInventoryItem, 0, len(catalog))}
	for _, definition := range catalog {
		item := ConfigurationInventoryItem{
			Definition:  definition,
			Environment: environment,
			Status:      domain.ConfigurationStatusNotConfigured,
		}
		if definition.Sensitive {
			item.Status = "not_managed"
		} else if state, ok := states[definition.Application+"\x00"+definition.Key]; ok && state.Latest != nil {
			item.Status = state.Latest.Status
			item.LatestVersion = state.Latest.Version
			item.LatestVersionStatus = state.Latest.Status
			item.Value = append(json.RawMessage(nil), state.Latest.Value...)
			if state.LastKnownGood != nil {
				item.LastKnownGoodVersion = state.LastKnownGood.Version
			}
		}
		inventory.Configurations = append(inventory.Configurations, item)
	}
	return inventory, nil
}

func (c *ConfigurationControlPlane) History(ctx context.Context, application, environment, key string) (domain.ConfigurationHistory, error) {
	definition, ok := findConfigurationDefinition(application, key)
	if !ok {
		return domain.ConfigurationHistory{}, invalidConfiguration("unknown application or configuration key")
	}
	if definition.Sensitive {
		return domain.ConfigurationHistory{}, invalidConfiguration("sensitive configuration values are managed outside this control plane")
	}
	environment, err := normalizeEnvironment(environment)
	if err != nil {
		return domain.ConfigurationHistory{}, err
	}
	return c.repository.GetConfigurationHistory(ctx, application, environment, key)
}

func (c *ConfigurationControlPlane) CreateVersion(ctx context.Context, request domain.ConfigurationVersionRequest) (domain.ConfigurationVersion, error) {
	definition, ok := findConfigurationDefinition(request.Application, request.Key)
	if !ok {
		return domain.ConfigurationVersion{}, invalidConfiguration("unknown application or configuration key")
	}
	if definition.Sensitive {
		return domain.ConfigurationVersion{}, invalidConfiguration("sensitive configuration values are managed outside this control plane")
	}
	if request.CreatedBy == uuid.Nil {
		return domain.ConfigurationVersion{}, invalidConfiguration("platform operator identity is required")
	}
	environment, err := normalizeEnvironment(request.Environment)
	if err != nil {
		return domain.ConfigurationVersion{}, err
	}
	request.Environment = environment
	if request.ExpectedVersion < 0 {
		return domain.ConfigurationVersion{}, invalidConfiguration("expected_version must be non-negative")
	}
	if request.RollbackOfVersionID != nil {
		if len(request.Value) != 0 {
			return domain.ConfigurationVersion{}, invalidConfiguration("value must be omitted when selecting a rollback version")
		}
		if *request.RollbackOfVersionID < 1 {
			return domain.ConfigurationVersion{}, invalidConfiguration("rollback_version_id must be positive")
		}
		history, err := c.repository.GetConfigurationHistory(ctx, request.Application, request.Environment, request.Key)
		if err != nil {
			return domain.ConfigurationVersion{}, fmt.Errorf("read rollback target: %w", err)
		}
		// Only the most recently acknowledged applied version is a rollback target.
		// Reports are ordered newest-first by the repository.
		var lastKnownGoodID int64
		for _, report := range history.Reports {
			if report.Status == domain.ConfigurationStatusApplied {
				lastKnownGoodID = report.VersionID
				break
			}
		}
		if lastKnownGoodID != *request.RollbackOfVersionID {
			return domain.ConfigurationVersion{}, invalidConfiguration("rollback_version_id must be the last-known-good version")
		}
		for _, version := range history.Versions {
			if version.ID == lastKnownGoodID {
				request.Value = append(json.RawMessage(nil), version.Value...)
				break
			}
		}
		if len(request.Value) == 0 {
			return domain.ConfigurationVersion{}, domain.ErrConfigurationVersionNotFound
		}
	}
	if err := validateConfigurationValue(definition, request.Value); err != nil {
		return domain.ConfigurationVersion{}, err
	}
	created, err := c.repository.CreateConfigurationVersion(ctx, request)
	if err != nil {
		return domain.ConfigurationVersion{}, err
	}
	created.Status = domain.ConfigurationStatusRequested
	return created, nil
}

func (c *ConfigurationControlPlane) RecordReport(ctx context.Context, request domain.ConfigurationReportRequest) (domain.ConfigurationReport, error) {
	if request.ReportedBy == uuid.Nil {
		return domain.ConfigurationReport{}, invalidConfiguration("platform operator identity is required")
	}
	if request.VersionID < 1 {
		return domain.ConfigurationReport{}, invalidConfiguration("version_id must be positive")
	}
	switch request.Status {
	case domain.ConfigurationStatusApplied, domain.ConfigurationStatusFailed, domain.ConfigurationStatusRollback:
	default:
		return domain.ConfigurationReport{}, invalidConfiguration("status must be applied, failed, or rollback")
	}
	return c.repository.RecordConfigurationReport(ctx, request)
}

func normalizeEnvironment(environment string) (string, error) {
	environment = strings.TrimSpace(environment)
	if environment == "" || len(environment) > 64 {
		return "", invalidConfiguration("environment is required and must be at most 64 characters")
	}
	for _, r := range environment {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return "", invalidConfiguration("environment contains invalid characters")
		}
	}
	return environment, nil
}

func validateConfigurationValue(definition domain.ConfigurationDefinition, value json.RawMessage) error {
	if len(value) == 0 || len(value) > 16*1024 || !utf8.Valid(value) {
		return invalidConfiguration("value is required, valid UTF-8, and at most 16384 bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return invalidConfiguration(fmt.Sprintf("value must be valid JSON: %v", err))
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return invalidConfiguration("value must contain exactly one JSON value")
	}
	if decoded == nil || definition.Type == "object" || definition.Type == "array" {
		return invalidConfiguration("value must be a non-null JSON scalar")
	}
	switch definition.Type {
	case "boolean":
		if _, ok := decoded.(bool); !ok {
			return invalidConfiguration("value must be a boolean")
		}
	case "integer":
		number, ok := decoded.(json.Number)
		if !ok {
			return invalidConfiguration("value must be an integer")
		}
		parsed, err := strconv.ParseInt(number.String(), 10, 64)
		if err != nil {
			return invalidConfiguration("value must be an integer")
		}
		if definition.Key == "REDIS_DB" {
			if parsed < 0 {
				return invalidConfiguration("value must be non-negative")
			}
		} else if parsed <= 0 {
			return invalidConfiguration("value must be positive")
		}
	case "url":
		text, ok := decoded.(string)
		if !ok {
			return invalidConfiguration("value must be a URL string")
		}
		if text == "" && !definition.Required {
			return nil
		}
		parsed, err := url.ParseRequestURI(text)
		if strings.TrimSpace(text) != text || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return invalidConfiguration("value must be an absolute HTTP(S) URL without user information")
		}
	case "string":
		text, ok := decoded.(string)
		if !ok || strings.TrimSpace(text) == "" && definition.Key != "TRUSTED_CLIENT_IP_HEADER" && definition.Key != "TRUSTED_PROXY_CIDRS" {
			return invalidConfiguration("value must be a non-empty string")
		}
		if definition.Key == "TRUSTED_CLIENT_IP_HEADER" && text != "" && !validHeaderName(text) {
			return invalidConfiguration("value must be a valid HTTP header name")
		}
		if definition.Key == "TRUSTED_PROXY_CIDRS" && strings.TrimSpace(text) != "" {
			for _, entry := range strings.Split(text, ",") {
				entry = strings.TrimSpace(entry)
				if entry == "" {
					return invalidConfiguration("value contains an empty proxy address")
				}
				if address, err := netip.ParseAddr(entry); err == nil {
					if address.Zone() != "" {
						return invalidConfiguration("proxy address must not have a zone")
					}
					continue
				}
				prefix, err := netip.ParsePrefix(entry)
				if err != nil || prefix.Bits() == 0 || prefix.Addr().Zone() != "" {
					return invalidConfiguration("value contains an invalid proxy IP or CIDR")
				}
			}
		}
	default:
		return invalidConfiguration("configuration key has unsupported value type")
	}
	return nil
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}
