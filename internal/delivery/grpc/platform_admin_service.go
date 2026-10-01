package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

const PlatformAdminCheckPath = "/tenant.PlatformAdminService/CheckActive"

type PlatformAdminServiceServer interface {
	CheckActive(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

type platformAdminServer struct{ auth *usecase.PlatformAuth }

func NewPlatformAdminServer(auth *usecase.PlatformAuth) PlatformAdminServiceServer {
	return &platformAdminServer{auth: auth}
}

func (s *platformAdminServer) CheckActive(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	id, err := uuid.Parse(req.GetFields()["user_id"].GetStringValue())
	version := req.GetFields()["platform_factor_version"].GetNumberValue()
	if err != nil || id == uuid.Nil || version < 1 || version != float64(int64(version)) {
		return structpb.NewStruct(map[string]interface{}{"allowed": false})
	}
	err = s.auth.CheckVersion(ctx, id, int64(version))
	if err != nil && !errors.Is(err, usecase.ErrPlatformForbidden) {
		return nil, status.Error(codes.Unavailable, "platform authorization unavailable")
	}
	return structpb.NewStruct(map[string]interface{}{"allowed": err == nil})
}

func RegisterPlatformAdminServiceServer(registrar grpc.ServiceRegistrar, server PlatformAdminServiceServer) {
	registrar.RegisterService(&grpc.ServiceDesc{
		ServiceName: "tenant.PlatformAdminService", HandlerType: (*PlatformAdminServiceServer)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "CheckActive", Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
			in := new(structpb.Struct)
			if err := dec(in); err != nil {
				return nil, err
			}
			call := func(ctx context.Context, request interface{}) (interface{}, error) {
				return srv.(PlatformAdminServiceServer).CheckActive(ctx, request.(*structpb.Struct))
			}
			if interceptor == nil {
				return call(ctx, in)
			}
			return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: PlatformAdminCheckPath}, call)
		}}}}, server)
}
