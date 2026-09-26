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
	CatalogPolicyServiceName                       = "tenant.CatalogPolicyService"
	CatalogPolicyServiceGetPublicCatalogPolicyPath = "/tenant.CatalogPolicyService/GetPublicCatalogPolicy"
)

// CatalogPolicyServiceServer serves the effective public catalog policy to
// academic (KEL-98). Like PermissionService it uses structpb because the
// repository carries generated protobuf files without their .proto source; the
// request is empty and the response carries "open", "applied_version", and
// "desired_version".
type CatalogPolicyServiceServer interface {
	GetPublicCatalogPolicy(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

func RegisterCatalogPolicyServiceServer(registrar grpc.ServiceRegistrar, server CatalogPolicyServiceServer) {
	registrar.RegisterService(&CatalogPolicyServiceDesc, server)
}

var CatalogPolicyServiceDesc = grpc.ServiceDesc{
	ServiceName: CatalogPolicyServiceName,
	HandlerType: (*CatalogPolicyServiceServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "GetPublicCatalogPolicy",
		Handler:    catalogPolicyServiceGetHandler,
	}},
	Streams:  []grpc.StreamDesc{},
	Metadata: "catalog-policy-contract",
}

func catalogPolicyServiceGetHandler(
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
		return srv.(CatalogPolicyServiceServer).GetPublicCatalogPolicy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: CatalogPolicyServiceGetPublicCatalogPolicyPath}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CatalogPolicyServiceServer).GetPublicCatalogPolicy(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

type catalogPolicyServer struct {
	policy domain.PublicCatalogPolicy
}

func NewCatalogPolicyServer(policy domain.PublicCatalogPolicy) CatalogPolicyServiceServer {
	return &catalogPolicyServer{policy: policy}
}

// GetPublicCatalogPolicy returns the effective applied policy. An evaluation
// failure is an Unavailable error, never a default answer, so academic fails
// closed instead of guessing that the catalog is open.
func (s *catalogPolicyServer) GetPublicCatalogPolicy(ctx context.Context, _ *structpb.Struct) (*structpb.Struct, error) {
	evaluated, err := s.policy.Evaluate(ctx)
	if err != nil {
		slog.Error("public catalog policy evaluation failed", "error", err)
		return nil, status.Error(codes.Unavailable, "public catalog policy unavailable")
	}
	return structpb.NewStruct(map[string]interface{}{
		"open":            evaluated.Open,
		"applied_version": float64(evaluated.AppliedVersion),
		"desired_version": float64(evaluated.DesiredVersion),
	})
}
