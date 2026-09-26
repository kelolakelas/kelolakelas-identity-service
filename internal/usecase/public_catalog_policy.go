package usecase

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// publicCatalogStateReader is the repository read that decides the effective
// public catalog policy. The Postgres configuration repository implements it.
type publicCatalogStateReader interface {
	PublicCatalogState(context.Context) (domain.PublicCatalogPolicyEvaluated, error)
}

var errPublicCatalogPolicyUnsupported = errors.New("public catalog policy store unavailable")

// versionedPublicCatalogPolicy implements domain.PublicCatalogPolicy on top of
// the KEL-96 configuration control plane (KEL-98), exactly like the KEL-97
// registration policy: the setting is an ordinary versioned configuration
// entry, so history, applied reports, and last-known-good rollback all keep
// working through the generic configuration endpoints. The effective value is
// the applied version, never merely the latest desired version.
type versionedPublicCatalogPolicy struct {
	repository domain.ConfigurationRepository
}

func NewPublicCatalogPolicy(repository domain.ConfigurationRepository) domain.PublicCatalogPolicy {
	return &versionedPublicCatalogPolicy{repository: repository}
}

// Evaluate fails closed: a repository that cannot read the effective state
// returns an error instead of a default, so a caller never sees Open=true
// unless the store positively decided it.
func (p *versionedPublicCatalogPolicy) Evaluate(ctx context.Context) (domain.PublicCatalogPolicyEvaluated, error) {
	reader, ok := p.repository.(publicCatalogStateReader)
	if !ok {
		return domain.PublicCatalogPolicyEvaluated{}, errPublicCatalogPolicyUnsupported
	}
	return reader.PublicCatalogState(ctx)
}

func (p *versionedPublicCatalogPolicy) Close(ctx context.Context, operator uuid.UUID) (domain.PublicCatalogPolicyEvaluated, error) {
	return p.createVersion(ctx, operator, false)
}

func (p *versionedPublicCatalogPolicy) Open(ctx context.Context, operator uuid.UUID) (domain.PublicCatalogPolicyEvaluated, error) {
	return p.createVersion(ctx, operator, true)
}

// createVersion appends a desired version guarded by the head it just read, so
// a concurrent change surfaces as ErrConfigurationVersionConflict. The new
// version does not take effect until an applied report is recorded through the
// regular control-plane report endpoint; the response describes the effective
// state at read time.
func (p *versionedPublicCatalogPolicy) createVersion(ctx context.Context, operator uuid.UUID, open bool) (domain.PublicCatalogPolicyEvaluated, error) {
	current, err := p.Evaluate(ctx)
	if err != nil {
		return domain.PublicCatalogPolicyEvaluated{}, err
	}
	value := json.RawMessage("false")
	if open {
		value = json.RawMessage("true")
	}
	_, err = p.repository.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{
		Application:     domain.PublicCatalogApplication,
		Environment:     domain.PublicCatalogEnvironment,
		Key:             domain.PublicCatalogOpenKey,
		ExpectedVersion: current.DesiredVersion,
		Value:           value,
		CreatedBy:       operator,
	})
	if err != nil {
		return domain.PublicCatalogPolicyEvaluated{}, err
	}
	return p.Evaluate(ctx)
}
