package grpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

const (
	PermissionServiceName                = "tenant.PermissionService"
	PermissionServiceCheckPermissionPath = "/tenant.PermissionService/CheckPermission"
)

// RequirePermissionTenantID controls whether CheckPermission rejects requests that omit
// tenant_id. ADR 0002 requires identity to accept tenant_id as optional first, so that an
// academic deployment still sending only role_id and permission keeps working, and only
// then to make it mandatory. Set it to false during the transition window (deploy identity,
// then academic) and to true once every caller sends tenant_id.
var RequirePermissionTenantID = false

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
//
// The decision is scoped to the tenant supplied in "tenant_id": the role must belong to
// that tenant or be a system role (tenant_id IS NULL), so a role lifted from another
// tenant never satisfies the check. A role that no longer exists yields no rows and is
// therefore denied.
//
// During the ADR 0002 transition window requests without "tenant_id" are still answered
// with the legacy global lookup so an academic deployment that has not been upgraded yet
// keeps working. Once every caller sends "tenant_id", set RequirePermissionTenantID to
// true to reject those requests instead.
//
// KEL-76 adds the optional "member_id" field. When it is present the request must also carry
// tenant_id, and "allowed" is true only for an active, not soft-deleted membership with that id
// in that tenant that currently carries role_id. Requests without member_id are answered exactly
// as before so academic and billing deployments that do not send it yet keep working.
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
	memberID, err := optionalMemberID(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenantValue, tenantErr := structString(req, "tenant_id")
	if tenantErr != nil && memberID != nil {
		// A membership is only meaningful inside a tenant. Never answer a member_id request
		// through the unscoped legacy lookup, which would ignore the membership entirely.
		return nil, status.Error(codes.InvalidArgument, "tenant_id is required when member_id is set")
	}
	if tenantErr != nil {
		if RequirePermissionTenantID {
			return nil, status.Error(codes.InvalidArgument, "tenant_id is required")
		}
		// Transition window: an academic deployment that has not been upgraded yet still
		// sends only role_id and permission, so fall back to the legacy global lookup.
		allowed, err := s.checkPermission(ctx, nil, parsedRoleID, permission)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("check permission: %v", err))
		}
		return structpb.NewStruct(map[string]interface{}{"allowed": allowed})
	}

	parsedTenantID, err := uuid.Parse(tenantValue)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "tenant_id must be a UUID")
	}

	if memberID != nil {
		// KEL-76: a caller that names the membership its token was minted for is only
		// allowed while that membership is active in this tenant and still carries the role.
		allowed, err := repository.ActiveMemberHasPermission(ctx, s.db, domain.MemberPermissionQuery{
			TenantID:   parsedTenantID,
			RoleID:     parsedRoleID,
			MemberID:   *memberID,
			Permission: permission,
		})
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("check permission: %v", err))
		}
		return structpb.NewStruct(map[string]interface{}{"allowed": allowed})
	}

	allowed, err := s.checkPermission(ctx, &parsedTenantID, parsedRoleID, permission)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("check permission: %v", err))
	}
	return structpb.NewStruct(map[string]interface{}{"allowed": allowed})
}

// checkPermission counts the role_permissions rows backing a decision. When tenantID is
// non-nil the role must belong to that tenant or be a system role (tenant_id IS NULL), so a
// role lifted from another tenant never satisfies the check. A role that no longer exists
// yields no rows and is therefore denied.
func (s *TenantServiceServer) checkPermission(ctx context.Context, tenantID *uuid.UUID, roleID uuid.UUID, permission string) (bool, error) {
	query := s.db.WithContext(ctx).
		Table("role_permissions rp").
		Joins("JOIN permissions p ON p.id = rp.permission_id").
		Joins("JOIN roles r ON r.id = rp.role_id").
		Where("r.id = ? AND p.name = ?", roleID, permission)
	if tenantID != nil {
		query = query.Where("r.tenant_id = ? OR r.tenant_id IS NULL", *tenantID)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// optionalMemberID reads the optional "member_id" field (KEL-76). An absent field or an empty
// string means the caller did not name a membership, which keeps the pre-KEL-76 behaviour for
// academic and billing deployments that do not send it yet; omitting the field grants nothing
// that sending it would not. A present value must be a UUID string.
func optionalMemberID(req *structpb.Struct) (*uuid.UUID, error) {
	value, ok := req.GetFields()["member_id"]
	if !ok || value == nil {
		return nil, nil
	}
	if _, isNull := value.GetKind().(*structpb.Value_NullValue); isNull {
		return nil, nil
	}
	if _, isString := value.GetKind().(*structpb.Value_StringValue); !isString {
		return nil, fmt.Errorf("member_id must be a UUID")
	}
	if value.GetStringValue() == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value.GetStringValue())
	if err != nil || parsed == uuid.Nil {
		return nil, fmt.Errorf("member_id must be a UUID")
	}
	return &parsed, nil
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
