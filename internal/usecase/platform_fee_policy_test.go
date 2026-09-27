package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// KEL-99: the generic configuration endpoint must refuse a fee policy value
// that billing could not parse, so an out-of-range rule never reaches history
// through the back door.
func TestPlatformFeePolicyGenericVersionValidation(t *testing.T) {
	controlPlane := NewConfigurationControlPlane(newMemoryConfigurationRepository())
	operator := uuid.New()
	create := func(expected int64, value string) error {
		_, err := controlPlane.CreateVersion(context.Background(), domain.ConfigurationVersionRequest{
			Application: domain.PlatformFeeApplication, Environment: domain.PlatformFeeEnvironment, Key: domain.PlatformFeePolicyKey,
			ExpectedVersion: expected, Value: json.RawMessage(value), CreatedBy: operator,
		})
		return err
	}
	for _, value := range []string{`"percent_bps=2001,fixed_fee=0"`, `"percent_bps=0,fixed_fee=50001"`, `"percent_bps=-1,fixed_fee=0"`, `"5%"`, `500`, `true`, `null`, `"fixed_fee=0,percent_bps=0"`} {
		if err := create(0, value); !IsConfigurationValidationError(err) {
			t.Fatalf("%s: err=%v, want invalid configuration", value, err)
		}
	}
	if err := create(0, `"percent_bps=2000,fixed_fee=50000"`); err != nil {
		t.Fatalf("valid maximum rejected: %v", err)
	}
}

func TestPlatformFeePolicySetValidatesAndGuardsHead(t *testing.T) {
	repository := &feeStateRepository{memoryConfigurationRepository: newMemoryConfigurationRepository()}
	policy := NewPlatformFeePolicy(repository)
	operator := uuid.New()
	if _, err := policy.Set(context.Background(), operator, domain.PlatformFeePolicyValue{PercentBps: 2001}); !errors.Is(err, domain.ErrInvalidPlatformFeePolicy) {
		t.Fatalf("out of range: %v", err)
	}
	if _, err := policy.Set(context.Background(), operator, domain.PlatformFeePolicyValue{PercentBps: 500, FixedFee: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Set(context.Background(), operator, domain.PlatformFeePolicyValue{PercentBps: 250, FixedFee: 0}); err != nil {
		t.Fatal(err)
	}
	history, err := repository.GetConfigurationHistory(context.Background(), domain.PlatformFeeApplication, domain.PlatformFeeEnvironment, domain.PlatformFeePolicyKey)
	if err != nil || len(history.Versions) != 2 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	for _, version := range history.Versions {
		var text string
		if err := json.Unmarshal(version.Value, &text); err != nil {
			t.Fatalf("stored value %s is not a JSON string", version.Value)
		}
		if _, err := domain.ParsePlatformFeePolicyValue(text); err != nil {
			t.Fatalf("stored value %q is not canonical", text)
		}
	}
	if repository.evaluations != 2 {
		t.Fatalf("Set must return the effective state, evaluations=%d", repository.evaluations)
	}
	unsupported := NewPlatformFeePolicy(newMemoryConfigurationRepository())
	if _, err := unsupported.Evaluate(context.Background()); err == nil {
		t.Fatal("a store without PlatformFeeState must fail closed")
	}
}

type feeStateRepository struct {
	*memoryConfigurationRepository
	evaluations int
}

func (r *feeStateRepository) PlatformFeeState(context.Context) (domain.PlatformFeePolicyEvaluated, error) {
	r.evaluations++
	return domain.PlatformFeePolicyEvaluated{Applied: true}, nil
}
