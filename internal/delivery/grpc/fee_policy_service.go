package grpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

const (
	FeePolicyServiceName                     = "tenant.FeePolicyService"
	FeePolicyServiceGetPlatformFeePolicyPath = "/tenant.FeePolicyService/GetPlatformFeePolicy"
)

// FeePolicyServiceServer serves the effective platform fee policy to billing
// (KEL-99) over the same structpb contract as CatalogPolicyService. The request
// is empty; the response carries "percent_bps", "fixed_fee",
// "applied_version", and "desired_version".
type FeePolicyServiceServer interface {
	GetPlatformFeePolicy(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

func RegisterFeePolicyServiceServer(registrar grpc.ServiceRegistrar, server FeePolicyServiceServer) {
	registrar.RegisterService(&FeePolicyServiceDesc, server)
}

var FeePolicyServiceDesc = grpc.ServiceDesc{
	ServiceName: FeePolicyServiceName,
	HandlerType: (*FeePolicyServiceServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "GetPlatformFeePolicy",
		Handler:    feePolicyServiceGetHandler,
	}},
	Streams:  []grpc.StreamDesc{},
	Metadata: "fee-policy-contract",
}

func feePolicyServiceGetHandler(
	srv interface{},
	ctx context.Context,
	dec func(interface{}) error,
	interceptor grpc.UnaryServerInterceptor,
) (interface{}, error) {
	in := new(structpb.Struct)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FeePolicyServiceServer).GetPlatformFeePolicy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: FeePolicyServiceGetPlatformFeePolicyPath}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FeePolicyServiceServer).GetPlatformFeePolicy(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

type feePolicyServer struct {
	policy domain.PlatformFeePolicy
}

func NewFeePolicyServer(policy domain.PlatformFeePolicy) FeePolicyServiceServer {
	return &feePolicyServer{policy: policy}
}

// GetPlatformFeePolicy returns the applied fee rule. An evaluation failure or a
// desired version that was never applied is an error, never a zero rule, so
// billing rejects the invoice instead of silently charging 0%.
func (s *feePolicyServer) GetPlatformFeePolicy(ctx context.Context, _ *structpb.Struct) (*structpb.Struct, error) {
	evaluated, err := s.policy.Evaluate(ctx)
	if err != nil {
		slog.Error("platform fee policy evaluation failed", "error", err)
		return nil, status.Error(codes.Unavailable, "platform fee policy unavailable")
	}
	if !evaluated.Applied {
		return nil, status.Error(codes.FailedPrecondition, "platform fee policy has no applied version")
	}
	return structpb.NewStruct(map[string]interface{}{
		"percent_bps":     float64(evaluated.PercentBps),
		"fixed_fee":       float64(evaluated.FixedFee),
		"applied_version": float64(evaluated.AppliedVersion),
		"desired_version": float64(evaluated.DesiredVersion),
	})
}
