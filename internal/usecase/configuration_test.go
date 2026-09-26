package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type memoryConfigurationRepository struct {
	mu       sync.Mutex
	versions []domain.ConfigurationVersion
	reports  []domain.ConfigurationReport
	states   map[string]domain.ConfigurationState
	fail     error
}

func newMemoryConfigurationRepository() *memoryConfigurationRepository {
	return &memoryConfigurationRepository{states: make(map[string]domain.ConfigurationState)}
}

func configurationScope(application, environment, key string) string {
	return application + "\x00" + environment + "\x00" + key
}

func (r *memoryConfigurationRepository) ListConfigurationState(_ context.Context, environment string) (map[string]domain.ConfigurationState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]domain.ConfigurationState, len(r.states))
	for _, state := range r.states {
		if state.Latest != nil && state.Latest.Environment == environment {
			result[state.Latest.Application+"\x00"+state.Latest.Key] = state
		}
	}
	return result, r.fail
}

func (r *memoryConfigurationRepository) GetConfigurationHistory(_ context.Context, application, environment, key string) (domain.ConfigurationHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result domain.ConfigurationHistory
	for index := len(r.reports) - 1; index >= 0; index-- {
		for _, version := range r.versions {
			if version.ID == r.reports[index].VersionID && version.Application == application && version.Environment == environment && version.Key == key {
				result.Reports = append(result.Reports, r.reports[index])
			}
		}
	}
	for _, version := range r.versions {
		if version.Application == application && version.Environment == environment && version.Key == key {
			result.Versions = append(result.Versions, version)
		}
	}
	return result, r.fail
}

func (r *memoryConfigurationRepository) CreateConfigurationVersion(_ context.Context, request domain.ConfigurationVersionRequest) (domain.ConfigurationVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return domain.ConfigurationVersion{}, r.fail
	}
	key := configurationScope(request.Application, request.Environment, request.Key)
	state := r.states[key]
	var current int64
	if state.Latest != nil {
		current = state.Latest.Version
	}
	if current != request.ExpectedVersion {
		return domain.ConfigurationVersion{}, domain.ErrConfigurationVersionConflict
	}
	version := domain.ConfigurationVersion{
		ID: int64(len(r.versions) + 1), Application: request.Application,
		Environment: request.Environment, Key: request.Key, Version: current + 1,
		Value: append(json.RawMessage(nil), request.Value...), CreatedBy: request.CreatedBy,
		CreatedAt: time.Now().UTC(), RollbackOfVersionID: request.RollbackOfVersionID,
		Status: domain.ConfigurationStatusRequested,
	}
	r.versions = append(r.versions, version)
	state.Latest = &version
	r.states[key] = state
	return version, nil
}

func (r *memoryConfigurationRepository) RecordConfigurationReport(_ context.Context, request domain.ConfigurationReportRequest) (domain.ConfigurationReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return domain.ConfigurationReport{}, r.fail
	}
	var version *domain.ConfigurationVersion
	for index := range r.versions {
		if r.versions[index].ID == request.VersionID {
			version = &r.versions[index]
			break
		}
	}
	if version == nil {
		return domain.ConfigurationReport{}, domain.ErrConfigurationVersionNotFound
	}
	report := domain.ConfigurationReport{ID: int64(len(r.reports) + 1), VersionID: request.VersionID, Status: request.Status, ReportedBy: request.ReportedBy, CreatedAt: time.Now().UTC()}
	r.reports = append(r.reports, report)
	key := configurationScope(version.Application, version.Environment, version.Key)
	state := r.states[key]
	if request.Status == domain.ConfigurationStatusApplied {
		applied := *version
		applied.Status = domain.ConfigurationStatusApplied
		state.LastKnownGood = &applied
	}
	if state.Latest != nil && state.Latest.ID == version.ID {
		latest := *version
		latest.Status = request.Status
		state.Latest = &latest
	}
	r.states[key] = state
	return report, nil
}

func TestConfigurationCatalogCoversEveryApplicationAndRedactsSensitiveSettings(t *testing.T) {
	catalog := ConfigurationCatalog()
	applications := map[string]int{}
	secrets := map[string]bool{}
	for _, definition := range catalog {
		applications[definition.Application]++
		if definition.Sensitive {
			secrets[definition.Application+"/"+definition.Key] = true
		}
	}
	for _, application := range []string{"web", "api-gateway", "identity", "academic", "billing"} {
		if applications[application] == 0 {
			t.Errorf("application %q has no configuration inventory", application)
		}
	}
	for _, key := range []string{
		"identity/DATABASE_URL", "identity/DB_PASSWORD", "identity/REDIS_PASSWORD", "identity/JWT_SECRET",
		"identity/RESEND_API_KEY", "identity/GOOGLE_MAPS_API_KEY", "academic/INTERNAL_SERVICE_CREDENTIAL",
		"billing/DUITKU_API_KEY", "billing/DUITKU_MERCHANT_CODE", "billing/RESEND_API_KEY",
	} {
		if !secrets[key] {
			t.Errorf("sensitive configuration key %q was not classified", key)
		}
	}
}

func TestConfigurationCatalogMatchesSourceInventoriesAndRedactsSensitiveDefaults(t *testing.T) {
	// Source inventories were checked against each application's config loader and
	// environment examples. Keep this contract test aligned when a consumer adds a key.
	expected := map[string]string{
		"web":         "GATEWAY_API_URL NEXT_PUBLIC_APP_URL AUTH_COOKIE_NAME TENANT_ID_COOKIE_NAME NODE_ENV",
		"api-gateway": "JWT_SECRET APP_URL PORT IDENTITY_SERVICE_URL ACADEMIC_SERVICE_URL BILLING_SERVICE_URL REDIS_HOST REDIS_PORT REDIS_USERNAME REDIS_PASSWORD REDIS_TLS REDIS_DB RATE_LIMIT_REQUESTS RATE_LIMIT_WINDOW_SECONDS RATE_LIMIT_PUBLIC_REQUESTS RATE_LIMIT_PROTECTED_REQUESTS RATE_LIMIT_LOGIN_REQUESTS RATE_LIMIT_REGISTER_REQUESTS RATE_LIMIT_WEBHOOK_REQUESTS RATE_LIMIT_WEBHOOK_WINDOW_SECONDS PROXY_UPSTREAM_TIMEOUT_SECONDS PROXY_MAX_BODY_BYTES SERVER_READ_HEADER_TIMEOUT_SECONDS SERVER_READ_TIMEOUT_SECONDS SERVER_WRITE_TIMEOUT_SECONDS SERVER_IDLE_TIMEOUT_SECONDS TRUSTED_PROXY_CIDRS TRUSTED_CLIENT_IP_HEADER",
		"identity":    "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME REDIS_HOST REDIS_PORT REDIS_USERNAME REDIS_PASSWORD REDIS_TLS REDIS_DB JWT_SECRET PORT APP_URL RESEND_API_KEY RESEND_FROM_EMAIL GOOGLE_MAPS_API_KEY GOOGLE_MAPS_GEOCODING_ENABLED GOOGLE_MAPS_TIMEOUT_SECONDS PERMISSION_REQUIRE_TENANT_ID TENANT_REGISTRATION_OPEN",
		"academic":    "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME IDENTITY_GRPC_HOST BILLING_SERVICE_URL INTERNAL_SERVICE_CREDENTIAL PORT JWT_SECRET CATALOG_TENANT_INFO_TTL_MINUTES CATALOG_TENANT_INFO_TIMEOUT_MS PUBLIC_CATALOG_OPEN CATALOG_POLICY_CACHE_TTL_SECONDS CATALOG_POLICY_TIMEOUT_MS",
		"billing":     "DATABASE_URL DB_HOST DB_PORT DB_SSLMODE DB_CHANNEL_BINDING DB_USER DB_PASSWORD DB_NAME PORT DUITKU_API_BASE_URL DUITKU_API_KEY DUITKU_MERCHANT_CODE DUITKU_CALLBACK_URL DUITKU_RETURN_URL ACADEMIC_SERVICE_URL INTERNAL_SERVICE_CREDENTIAL JWT_SECRET SUBSCRIPTION_WORKER_ENABLED SUBSCRIPTION_WORKER_INTERVAL_MINUTES SUBSCRIPTION_PAYMENT_REMINDER_INTERVAL_DAYS SUBSCRIPTION_PAYMENT_EXPIRY_PERIOD_DAYS PAYMENT_RECONCILIATION_WORKER_ENABLED PAYMENT_RECONCILIATION_WORKER_INTERVAL_MINUTES PAYMENT_RECONCILIATION_MAX_ATTEMPTS TRANSACTION_EXPIRY_WORKER_ENABLED TRANSACTION_EXPIRY_WORKER_INTERVAL_MINUTES TRANSACTION_CLAIM_TIMEOUT_MINUTES RESEND_API_KEY RESEND_FROM_EMAIL IDENTITY_GRPC_HOST IDENTITY_PERMISSION_TIMEOUT_MS",
	}
	got := make(map[string]map[string]bool, len(expected))
	for _, definition := range ConfigurationCatalog() {
		if got[definition.Application] == nil {
			got[definition.Application] = make(map[string]bool)
		}
		got[definition.Application][definition.Key] = true
		if definition.Sensitive && definition.Default != "" {
			t.Errorf("sensitive default leaked for %s/%s", definition.Application, definition.Key)
		}
	}
	for application, keys := range expected {
		for _, key := range strings.Fields(keys) {
			if !got[application][key] {
				t.Errorf("source key %s/%s is missing from configuration inventory", application, key)
			}
		}
		if len(got[application]) != len(strings.Fields(keys)) {
			t.Errorf("inventory for %s has %d keys; source snapshot has %d", application, len(got[application]), len(strings.Fields(keys)))
		}
	}
}

func TestConfigurationCatalogApplicationAndValidationMetadata(t *testing.T) {
	cases := []struct {
		application, key, method, validation string
		bootstrap, sensitive                 bool
	}{
		{"api-gateway", "RATE_LIMIT_REQUESTS", "restart", "positive integer", false, false},
		{"api-gateway", "TRUSTED_PROXY_CIDRS", "restart", "comma-separated IP/CIDR", true, false},
		{"identity", "JWT_SECRET", "rotation", "deployment-only: supply and validate in the secret manager; control plane rejects values", true, true},
		{"identity", "DATABASE_URL", "rotation", "deployment-only: supply and validate in the secret manager; control plane rejects values", true, true},
		{"web", "NEXT_PUBLIC_APP_URL", "rebuild/redeploy", "absolute HTTP(S) string", true, false},
	}
	for _, test := range cases {
		definition, ok := findConfigurationDefinition(test.application, test.key)
		if !ok || definition.Owner != test.application || definition.ApplicationMethod != test.method || !strings.Contains(definition.Validation, test.validation) || definition.BootstrapOnly != test.bootstrap || definition.Sensitive != test.sensitive {
			t.Errorf("%s/%s metadata = %+v (found=%v)", test.application, test.key, definition, ok)
		}
	}
	for _, definition := range ConfigurationCatalog() {
		if definition.Owner == "" || definition.Type == "" || definition.Validation == "" || definition.ApplicationMethod == "" {
			t.Errorf("incomplete metadata for %s/%s", definition.Application, definition.Key)
		}
		if definition.ApplicationMethod == "dynamic" {
			t.Errorf("%s/%s claims unsupported dynamic application", definition.Application, definition.Key)
		}
	}
	inventory, err := NewConfigurationControlPlane(newMemoryConfigurationRepository()).Inventory(context.Background(), "staging")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Configurations []struct {
			Definition map[string]json.RawMessage `json:"definition"`
		} `json:"configurations"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Configurations) != len(ConfigurationCatalog()) {
		t.Fatalf("serialized inventory has %d entries", len(decoded.Configurations))
	}
	for _, entry := range decoded.Configurations {
		for _, field := range []string{"owner", "application", "key", "type", "default", "sensitive", "validation", "application_method", "bootstrap_only"} {
			if _, ok := entry.Definition[field]; !ok {
				t.Fatalf("serialized inventory missing %s: %s", field, entry.Definition)
			}
		}
	}
}

func TestConfigurationVersionValidationAndOptimisticConflict(t *testing.T) {
	repository := newMemoryConfigurationRepository()
	controlPlane := NewConfigurationControlPlane(repository)
	operator := uuid.New()
	create := func(key string, expected int64, value string) (domain.ConfigurationVersion, error) {
		return controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{
			Application: "api-gateway", Environment: "staging", Key: key,
			ExpectedVersion: expected, Value: json.RawMessage(value), CreatedBy: operator,
		})
	}
	version, err := create("RATE_LIMIT_REQUESTS", 0, "75")
	if err != nil || version.Version != 1 || version.Status != domain.ConfigurationStatusRequested {
		t.Fatalf("version=%+v error=%v", version, err)
	}
	if _, err := create("RATE_LIMIT_REQUESTS", 0, "80"); !errors.Is(err, domain.ErrConfigurationVersionConflict) {
		t.Fatalf("stale version error=%v, want conflict", err)
	}
	for _, test := range []struct{ key, value string }{
		{"UNKNOWN_KEY", "1"}, {"JWT_SECRET", `"plaintext-secret"`},
		{"RATE_LIMIT_REQUESTS", `"not-an-integer"`}, {"RATE_LIMIT_REQUESTS", "0"},
		{"APP_URL", `"ftp://example.test"`}, {"TRUSTED_CLIENT_IP_HEADER", `"X Bad Header"`},
		{"TRUSTED_PROXY_CIDRS", `"0.0.0.0/0"`}, {"REDIS_TLS", "null"},
		{"RATE_LIMIT_REQUESTS", "1 2"},
	} {
		t.Run(test.key+"/"+test.value, func(t *testing.T) {
			if _, err := create(test.key, 0, test.value); err == nil {
				t.Fatal("expected invalid configuration to be rejected")
			}
		})
	}
}

func TestConfigurationCatalogRedactsSecretsAndReportsLastKnownGood(t *testing.T) {
	repository := newMemoryConfigurationRepository()
	controlPlane := NewConfigurationControlPlane(repository)
	operator := uuid.New()
	secretState := domain.ConfigurationVersion{ID: 9, Application: "identity", Environment: "prod", Key: "JWT_SECRET", Version: 1, Value: json.RawMessage(`"must-not-leak"`)}
	repository.states[configurationScope("identity", "prod", "JWT_SECRET")] = domain.ConfigurationState{Latest: &secretState}

	first, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "prod", Key: "RATE_LIMIT_REQUESTS", Value: json.RawMessage("60"), CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlPlane.RecordReport(context.Background(), domain.ConfigurationReportRequest{VersionID: first.ID, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); err != nil {
		t.Fatal(err)
	}
	second, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "prod", Key: "RATE_LIMIT_REQUESTS", ExpectedVersion: 1, Value: json.RawMessage("90"), CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlPlane.RecordReport(context.Background(), domain.ConfigurationReportRequest{VersionID: second.ID, Status: domain.ConfigurationStatusFailed, ReportedBy: operator}); err != nil {
		t.Fatal(err)
	}
	inventory, err := controlPlane.Inventory(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inventory.Configurations {
		if item.Definition.Key == "JWT_SECRET" && item.Definition.Application == "identity" {
			if item.Value != nil || item.LatestVersion != 0 || item.Status != "not_managed" {
				t.Fatalf("sensitive inventory leaked or exposed management status: %+v", item)
			}
		}
		if item.Definition.Application == "api-gateway" && item.Definition.Key == "RATE_LIMIT_REQUESTS" {
			if item.Status != domain.ConfigurationStatusFailed || item.LastKnownGoodVersion != 1 || item.LatestVersion != 2 {
				t.Fatalf("failed/latest and last-known-good status mismatch: %+v", item)
			}
		}
	}
	if len(repository.reports) != 2 {
		t.Fatalf("record-only reports=%d, want 2", len(repository.reports))
	}
}

func TestConfigurationManualRollbackCreatesNewRequestedVersion(t *testing.T) {
	repository := newMemoryConfigurationRepository()
	controlPlane := NewConfigurationControlPlane(repository)
	operator := uuid.New()
	first, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "staging", Key: "RATE_LIMIT_REQUESTS", Value: json.RawMessage("60"), CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlPlane.RecordReport(context.Background(), domain.ConfigurationReportRequest{VersionID: first.ID, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); err != nil {
		t.Fatal(err)
	}
	if _, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "staging", Key: "RATE_LIMIT_REQUESTS", ExpectedVersion: 1, Value: json.RawMessage("80"), CreatedBy: operator}); err != nil {
		t.Fatal(err)
	}
	rollbackTarget := first.ID
	rollback, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "staging", Key: "RATE_LIMIT_REQUESTS", ExpectedVersion: 2, RollbackOfVersionID: &rollbackTarget, CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	if string(rollback.Value) != "60" || rollback.Status != domain.ConfigurationStatusRequested || rollback.RollbackOfVersionID == nil {
		t.Fatalf("rollback version=%+v; expected copied value as a new requested version", rollback)
	}
	otherEnvironment, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "prod", Key: "RATE_LIMIT_REQUESTS", Value: json.RawMessage("99"), CreatedBy: operator})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []int64{rollback.ID, otherEnvironment.ID, 999} {
		_, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "staging", Key: "RATE_LIMIT_REQUESTS", ExpectedVersion: 3, RollbackOfVersionID: &target, CreatedBy: operator})
		if err == nil {
			t.Fatalf("non-applied or cross-environment target %d was accepted", target)
		}
	}
	_, err = controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "staging", Key: "RATE_LIMIT_REQUESTS", ExpectedVersion: 3, RollbackOfVersionID: &rollbackTarget, Value: json.RawMessage("999"), CreatedBy: operator})
	if !IsConfigurationValidationError(err) {
		t.Fatalf("rollback with a conflicting supplied value: %v", err)
	}
	if _, err := controlPlane.RecordReport(context.Background(), domain.ConfigurationReportRequest{VersionID: 999, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); !errors.Is(err, domain.ErrConfigurationVersionNotFound) {
		t.Fatalf("unknown version report error=%v", err)
	}
}

func TestConfigurationRepositoryFailureDoesNotReportSuccess(t *testing.T) {
	repository := newMemoryConfigurationRepository()
	repository.fail = fmt.Errorf("database unavailable")
	controlPlane := NewConfigurationControlPlane(repository)
	if _, err := controlPlane.Inventory(context.Background(), "prod"); err == nil {
		t.Fatal("inventory must return persistence failure")
	}
	if _, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{Application: "api-gateway", Environment: "prod", Key: "RATE_LIMIT_REQUESTS", Value: json.RawMessage("50"), CreatedBy: uuid.New()}); err == nil {
		t.Fatal("version creation must return persistence failure")
	}
}
