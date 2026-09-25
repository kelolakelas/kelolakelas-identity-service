package usecase

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// versionedRegistrationPolicy implements domain.RegistrationPolicy on top of
// the KEL-96 configuration control plane. There is no second state store: the
// setting is an ordinary versioned configuration entry, so desired versions,
// applied acknowledgements, history, and rollback all keep working unchanged.
//
// The effective value is the applied version, not the latest desired version.
// An operator who requests a version without acknowledging it as applied in
// the control plane has not decided the runtime policy yet; the last applied
// acknowledgement does. A read failure, a missing applied version, or an
// undesired change fail closed per the KEL-97 contract.
type versionedRegistrationPolicy struct {
	repository domain.ConfigurationRepository
}

func NewRegistrationPolicy(repository domain.ConfigurationRepository) domain.RegistrationPolicy {
	return &versionedRegistrationPolicy{repository: repository}
}

func (p *versionedRegistrationPolicy) Evaluate(ctx context.Context) (domain.RegistrationPolicyEvaluated, error) {
	if reader, ok := p.repository.(interface {
		RegistrationState(context.Context) (domain.RegistrationPolicyEvaluated, error)
	}); ok {
		return reader.RegistrationState(ctx)
	}
	states, err := p.repository.ListConfigurationState(ctx, domain.TenantRegistrationEnvironment)
	if err != nil {
		return domain.RegistrationPolicyEvaluated{}, err
	}
	state := states[domain.TenantOnboardingApplication+"\x00"+domain.TenantRegistrationOpenKey]
	evaluated := domain.RegistrationPolicyEvaluated{
		Application: domain.TenantOnboardingApplication,
		Key:         domain.TenantRegistrationOpenKey,
		Environment: domain.TenantRegistrationEnvironment,
	}
	if state.Latest != nil {
		evaluated.DesiredVersion = state.Latest.Version
	}
	open, err := domain.RegistrationOpenFromApplied(&state)
	if err != nil {
		return evaluated, err
	}
	evaluated.Open = open
	if state.LastKnownGood != nil {
		evaluated.AppliedVersion = state.LastKnownGood.Version
	}
	return evaluated, nil
}

func (p *versionedRegistrationPolicy) Close(ctx context.Context, operator uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	return p.createVersion(ctx, operator, false)
}

func (p *versionedRegistrationPolicy) Open(ctx context.Context, operator uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	return p.createVersion(ctx, operator, true)
}

// createVersion appends a desired version, then reads the effective policy.
// The new version does not become effective until an applied report is recorded
// through the regular control-plane report endpoint.
func (p *versionedRegistrationPolicy) createVersion(ctx context.Context, operator uuid.UUID, open bool) (domain.RegistrationPolicyEvaluated, error) {
	states, err := p.repository.ListConfigurationState(ctx, domain.TenantRegistrationEnvironment)
	if err != nil {
		return domain.RegistrationPolicyEvaluated{}, err
	}
	state := states[domain.TenantOnboardingApplication+"\x00"+domain.TenantRegistrationOpenKey]
	expectedVersion := int64(0)
	if state.Latest != nil {
		expectedVersion = state.Latest.Version
	}
	value := json.RawMessage("false")
	if open {
		value = json.RawMessage("true")
	}
	_, err = p.repository.CreateConfigurationVersion(ctx, domain.ConfigurationVersionRequest{
		Application:     domain.TenantOnboardingApplication,
		Environment:     domain.TenantRegistrationEnvironment,
		Key:             domain.TenantRegistrationOpenKey,
		ExpectedVersion: expectedVersion,
		Value:           value,
		CreatedBy:       operator,
	})
	if err != nil {
		return domain.RegistrationPolicyEvaluated{}, err
	}
	// Evaluate through the same path as GET and registration. A report (or a
	// concurrent policy mutation) can arrive after creation, so the response
	// must describe the effective state at read time rather than the request.
	return p.Evaluate(ctx)
}
