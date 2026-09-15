package grpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	PermissionServiceName                = "tenant.PermissionService"
	PermissionServiceCheckPermissionPath = "/tenant.PermissionService/CheckPermission"
)

// PermissionServiceServer is the internal identity-to-domain authorization contract.
// structpb is used because the repository carries generated protobuf files without
// their source .proto; the fields are documented and validated at this seam.
type PermissionServiceServer interface {
	CheckPermission(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

func RegisterPermissionServiceServer(registrar grpc.ServiceRegistrar, server PermissionServiceServer) {
	registrar.RegisterService(&PermissionServiceDesc, server)
}

var PermissionServiceDesc = grpc.ServiceDesc{
	ServiceName: PermissionServiceName,
	HandlerType: (*PermissionServiceServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "CheckPermission",
		Handler:    permissionServiceCheckPermissionHandler,
	}},
	Streams:  []grpc.StreamDesc{},
	Metadata: "permission-contract",
}

func permissionServiceCheckPermissionHandler(
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
		return srv.(PermissionServiceServer).CheckPermission(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: PermissionServiceCheckPermissionPath}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PermissionServiceServer).CheckPermission(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

// CheckPermission answers from persisted role_permissions so role changes take
// effect immediately even when the caller presents an older JWT.
func (s *TenantServiceServer) CheckPermission(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	roleID, err := structString(req, "role_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	permission, err := structString(req, "permission")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	parsedRoleID, err := uuid.Parse(roleID)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "role_id must be a UUID")
	}

	var count int64
	err = s.db.WithContext(ctx).
		Table("role_permissions rp").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Joins("JOIN roles r ON r.id = rp.role_id").
		Where("r.id = ? AND p.name = ?", parsedRoleID, permission).
		Count(&count).Error
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("check permission: %v", err))
	}
	return structpb.NewStruct(map[string]interface{}{"allowed": count > 0})
}

func structString(req *structpb.Struct, key string) (string, error) {
	if req == nil || req.GetFields()[key] == nil {
		return "", fmt.Errorf("%s is required", key)
	}
	value := req.GetFields()[key].GetStringValue()
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}
