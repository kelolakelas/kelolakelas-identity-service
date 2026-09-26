package grpc

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"testing"
)

type catalogPolicyStub struct {
	state domain.PublicCatalogPolicyEvaluated
	err   error
}

func (s *catalogPolicyStub) Evaluate(context.Context) (domain.PublicCatalogPolicyEvaluated, error) {
	return s.state, s.err
}
func (s *catalogPolicyStub) Close(context.Context, uuid.UUID) (domain.PublicCatalogPolicyEvaluated, error) {
	return s.state, s.err
}
func (s *catalogPolicyStub) Open(context.Context, uuid.UUID) (domain.PublicCatalogPolicyEvaluated, error) {
	return s.state, s.err
}

func TestCatalogPolicyServiceEffectiveVersionAndOutage(t *testing.T) {
	stub := &catalogPolicyStub{state: domain.PublicCatalogPolicyEvaluated{Open: false, AppliedVersion: 2, DesiredVersion: 3}}
	server := NewCatalogPolicyServer(stub)
	response, err := server.GetPublicCatalogPolicy(context.Background(), &structpb.Struct{})
	if err != nil || response.Fields["open"].GetBoolValue() || response.Fields["applied_version"].GetNumberValue() != 2 || response.Fields["desired_version"].GetNumberValue() != 3 {
		t.Fatalf("response=%v err=%v", response, err)
	}
	stub.err = errors.New("database down")
	response, err = server.GetPublicCatalogPolicy(context.Background(), &structpb.Struct{})
	if response != nil || status.Code(err) != codes.Unavailable {
		t.Fatalf("response=%v err=%v", response, err)
	}
}
