package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type feePolicyStub struct {
	state domain.PlatformFeePolicyEvaluated
	err   error
}

func (s *feePolicyStub) Evaluate(context.Context) (domain.PlatformFeePolicyEvaluated, error) {
	return s.state, s.err
}
func (s *feePolicyStub) Set(context.Context, uuid.UUID, domain.PlatformFeePolicyValue) (domain.PlatformFeePolicyEvaluated, error) {
	return s.state, s.err
}

func TestFeePolicyServiceAppliedRuleAndFailClosed(t *testing.T) {
	stub := &feePolicyStub{state: domain.PlatformFeePolicyEvaluated{Applied: true, PercentBps: 500, FixedFee: 1000, AppliedVersion: 2, DesiredVersion: 3}}
	server := NewFeePolicyServer(stub)
	response, err := server.GetPlatformFeePolicy(context.Background(), &structpb.Struct{})
	if err != nil || response.Fields["percent_bps"].GetNumberValue() != 500 || response.Fields["fixed_fee"].GetNumberValue() != 1000 ||
		response.Fields["applied_version"].GetNumberValue() != 2 || response.Fields["desired_version"].GetNumberValue() != 3 {
		t.Fatalf("response=%v err=%v", response, err)
	}
	stub.state = domain.PlatformFeePolicyEvaluated{Applied: false, DesiredVersion: 1}
	response, err = server.GetPlatformFeePolicy(context.Background(), &structpb.Struct{})
	if response != nil || status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unapplied response=%v err=%v", response, err)
	}
	stub.err = errors.New("database down")
	response, err = server.GetPlatformFeePolicy(context.Background(), &structpb.Struct{})
	if response != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("outage response=%v err=%v", response, err)
	}
}
