package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

func appliedRegistrationState(appliedValue string, appliedVersion int64, desiredVersion int64, hasDesired bool) *memoryConfigurationRepository {
	repository := newMemoryConfigurationRepository()
	applied := domain.ConfigurationVersion{
		ID: appliedVersion, Application: domain.TenantOnboardingApplication,
		Environment: domain.TenantRegistrationEnvironment, Key: domain.TenantRegistrationOpenKey,
		Version: appliedVersion, Value: json.RawMessage(appliedValue), CreatedBy: uuid.New(),
	}
	key := configurationScope(domain.TenantOnboardingApplication, domain.TenantRegistrationEnvironment, domain.TenantRegistrationOpenKey)
	state := repository.states[key]
	appliedCopy := applied
	appliedCopy.Status = domain.ConfigurationStatusApplied
	state.LastKnownGood = &appliedCopy
	if hasDesired {
		desired := applied
		desired.Version = desiredVersion
		desired.Status = domain.ConfigurationStatusRequested
		state.Latest = &desired
	} else {
		latest := applied
		latest.Status = domain.ConfigurationStatusApplied
		state.Latest = &latest
	}
	repository.states[key] = state
	return repository
}

func TestRegistrationOpenFromAppliedFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		state     *domain.ConfigurationState
		wantOpen  bool
		wantError bool
	}{
		{name: "nil state", state: nil, wantOpen: false, wantError: true},
		{name: "no applied version", state: &domain.ConfigurationState{Latest: &domain.ConfigurationVersion{Version: 3}}, wantOpen: false, wantError: true},
		{name: "applied true", state: &domain.ConfigurationState{LastKnownGood: &domain.ConfigurationVersion{Version: 1, Value: json.RawMessage("true")}}, wantOpen: true},
		{name: "applied false", state: &domain.ConfigurationState{LastKnownGood: &domain.ConfigurationVersion{Version: 2, Value: json.RawMessage("false")}}, wantOpen: false},
		{name: "applied null", state: &domain.ConfigurationState{LastKnownGood: &domain.ConfigurationVersion{Value: json.RawMessage("null")}}, wantOpen: false, wantError: true},
		{name: "applied non-boolean", state: &domain.ConfigurationState{LastKnownGood: &domain.ConfigurationVersion{Value: json.RawMessage(`"yes"`)}}, wantOpen: false, wantError: true},
		{name: "applied empty", state: &domain.ConfigurationState{LastKnownGood: &domain.ConfigurationVersion{Value: json.RawMessage("  ")}}, wantOpen: false, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			open, err := domain.RegistrationOpenFromApplied(tc.state)
			if open != tc.wantOpen {
				t.Fatalf("open=%t want=%t err=%v", open, tc.wantOpen, err)
			}
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v wantError=%t", err, tc.wantError)
			}
		})
	}
}

func TestVersionedRegistrationPolicyEvaluateReadsAppliedValue(t *testing.T) {
	tests := []struct {
		name        string
		repository  *memoryConfigurationRepository
		repoFail    error
		wantOpen    bool
		wantError   bool
		wantApplied int64
		wantDesired int64
	}{
		{
			name:        "seeded applied open",
			repository:  appliedRegistrationState("true", 1, 1, false),
			wantOpen:    true,
			wantApplied: 1,
			wantDesired: 1,
		},
		{
			name:        "unacknowledged close does not close registration",
			repository:  appliedRegistrationState("true", 1, 2, true),
			wantOpen:    true,
			wantApplied: 1,
			wantDesired: 2,
		},
		{
			name:        "applied closed",
			repository:  appliedRegistrationState("false", 3, 3, false),
			wantOpen:    false,
			wantApplied: 3,
			wantDesired: 3,
		},
		{
			name:       "setting absent fails closed",
			repository: newMemoryConfigurationRepository(),
			wantOpen:   false,
			wantError:  true,
		},
		{
			name: "store outage fails closed",
			repository: func() *memoryConfigurationRepository {
				r := appliedRegistrationState("true", 1, 1, false)
				r.fail = errors.New("store down")
				return r
			}(),
			repoFail:  errors.New("store down"),
			wantOpen:  false,
			wantError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evaluated, err := NewRegistrationPolicy(tc.repository).Evaluate(context.Background())
			if (err != nil) != tc.wantError {
				t.Fatalf("err=%v wantError=%t", err, tc.wantError)
			}
			if evaluated.Open != tc.wantOpen {
				t.Fatalf("open=%t want=%t (err=%v)", evaluated.Open, tc.wantOpen, err)
			}
			if err == nil {
				if evaluated.AppliedVersion != tc.wantApplied || evaluated.DesiredVersion != tc.wantDesired {
					t.Fatalf("applied=%d desired=%d want %d/%d", evaluated.AppliedVersion, evaluated.DesiredVersion, tc.wantApplied, tc.wantDesired)
				}
				if evaluated.Application != domain.TenantOnboardingApplication || evaluated.Key != domain.TenantRegistrationOpenKey {
					t.Fatalf("identity=%s/%s", evaluated.Application, evaluated.Key)
				}
			}
		})
	}
}

func TestVersionedRegistrationPolicyCloseAndOpenAppendVersions(t *testing.T) {
	repository := appliedRegistrationState("true", 1, 1, false)
	policy := NewRegistrationPolicy(repository)
	operator := uuid.New()

	closed, err := policy.Close(context.Background(), operator)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed.Open {
		t.Fatalf("close reported open=true: %+v", closed)
	}
	if closed.DesiredVersion != 2 {
		t.Fatalf("desired=%d want 2", closed.DesiredVersion)
	}
	// A requested close must not take effect until it is acknowledged applied.
	evaluated, err := policy.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("evaluate after close: %v", err)
	}
	if !evaluated.Open {
		t.Fatal("unacknowledged close already closed registration")
	}
	versionID := int64(0)
	for _, version := range repository.versions {
		if version.Version == closed.DesiredVersion {
			versionID = version.ID
		}
	}
	if versionID == 0 {
		t.Fatalf("close version %d missing from repository", closed.DesiredVersion)
	}
	if _, err := repository.RecordConfigurationReport(context.Background(), domain.ConfigurationReportRequest{VersionID: versionID, Status: domain.ConfigurationStatusApplied, ReportedBy: operator}); err != nil {
		t.Fatalf("acknowledge close: %v", err)
	}
	evaluated, err = policy.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("evaluate after acknowledgement: %v", err)
	}
	if evaluated.Open {
		t.Fatal("acknowledged close did not close registration")
	}
	if evaluated.AppliedVersion != 2 {
		t.Fatalf("applied=%d want 2", evaluated.AppliedVersion)
	}

	opened, err := policy.Open(context.Background(), operator)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !opened.Open || opened.DesiredVersion != 3 {
		t.Fatalf("open=%+v want desired 3", opened)
	}
}

func TestVersionedRegistrationPolicyConcurrentMutationsNeverLoseUpdates(t *testing.T) {
	repository := appliedRegistrationState("true", 1, 1, false)
	policy := NewRegistrationPolicy(repository)

	const mutations = 8
	var wg sync.WaitGroup
	results := make(chan error, mutations)
	for i := 0; i < mutations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := policy.Close(context.Background(), uuid.New())
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrConfigurationVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes == 0 || successes+conflicts != mutations {
		t.Fatalf("successes=%d conflicts=%d for %d mutations", successes, conflicts, mutations)
	}
	evaluated, err := policy.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	// The head advances exactly once per accepted mutation: no lost update.
	if evaluated.DesiredVersion != 1+int64(successes) {
		t.Fatalf("desired=%d want %d (successes=%d)", evaluated.DesiredVersion, 1+int64(successes), successes)
	}
}
