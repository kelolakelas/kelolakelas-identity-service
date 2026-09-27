package usecase

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// platformFeeStateReader is the repository read that decides the effective
// platform fee policy. The Postgres configuration repository implements it.
type platformFeeStateReader interface {
	PlatformFeeState(context.Context) (domain.PlatformFeePolicyEvaluated, error)
}

var errPlatformFeePolicyUnsupported = errors.New("platform fee policy store unavailable")

// versionedPlatformFeePolicy implements domain.PlatformFeePolicy on the KEL-96
// configuration control plane (KEL-99), like the registration and public
// catalog policies: the rule is an ordinary versioned configuration entry, so
// history, applied reports, and last-known-good rollback keep working through
// the generic configuration endpoints. The effective rule is the applied
// version, never merely the latest desired version.
type versionedPlatformFeePolicy struct {
	repository domain.ConfigurationRepository
}

func NewPlatformFeePolicy(repository domain.ConfigurationRepository) domain.PlatformFeePolicy {
	return &versionedPlatformFeePolicy{repository: repository}
}

// Evaluate fails closed: a store that cannot decide the applied rule returns an
// error, never a zero rule.
func (p *versionedPlatformFeePolicy) Evaluate(ctx context.Context) (domain.PlatformFeePolicyEvaluated, error) {
	reader, ok := p.repository.(platformFeeStateReader)
	if !ok {
		return domain.PlatformFeePolicyEvaluated{}, errPlatformFeePolicyUnsupported
	}
	return reader.PlatformFeeState(ctx)
}

// Set appends a desired version guarded by the head it just read, so a
// concurrent change surfaces as ErrConfigurationVersionConflict. Out-of-range
// values are rejected before anything is written. The version takes effect for
// new transactions only once an applied report is recorded; the response
// describes the effective state at read time.
//
// The head is read directly rather than through Evaluate so an operator can
// still write a corrective version while the current desired version has not
// been applied yet.
func (p *versionedPlatformFeePolicy) Set(ctx context.Context, operator uuid.UUID, value domain.PlatformFeePolicyValue) (domain.PlatformFeePolicyEvaluated, error) {
	if err := value.Validate(); err != nil {
		return domain.PlatformFeePolicyEvaluated{}, err
	}
	history, err := p.repository.GetConfigurationHistory(ctx, domain.PlatformFeeApplication, domain.PlatformFeeEnvironment, domain.PlatformFeePolicyKey)
	if err != nil {
		return domain.PlatformFeePolicyEvaluated{}, err
	}
	var expected int64
	for _, version := range history.Versions {
		if version.Version > expected {
			expected = version.Version
		}
	}
	encoded, err := json.Marshal(value.Encode())
	if err != nil {
		return domain.PlatformFeePolicyEvaluated{}, err
	}
	if _, err = p.repository.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{
		Application:     domain.PlatformFeeApplication,
		Environment:     domain.PlatformFeeEnvironment,
		Key:             domain.PlatformFeePolicyKey,
		ExpectedVersion: expected,
		Value:           encoded,
		CreatedBy:       operator,
	}); err != nil {
		return domain.PlatformFeePolicyEvaluated{}, err
	}
	return p.Evaluate(ctx)
}
