package grpc

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
)

const (
	MembershipServiceName                  = "tenant.MembershipService"
	MembershipServiceCheckActiveMemberPath = "/tenant.MembershipService/CheckActiveMembership"
)

// MembershipServiceServer answers whether a member row is active in a tenant,
// so academic can validate a substitute tutor before assigning sessions to
// them (KEL-135). Like PermissionService it uses structpb because the
// repository carries generated protobuf files without their source .proto.
type MembershipServiceServer interface {
	CheckActiveMembership(context.Context, *structpb.Struct) (*structpb.Struct, error)
}

func RegisterMembershipServiceServer(registrar grpc.ServiceRegistrar, server MembershipServiceServer) {
	registrar.RegisterService(&MembershipServiceDesc, server)
}

var MembershipServiceDesc = grpc.ServiceDesc{
	ServiceName: MembershipServiceName,
	HandlerType: (*MembershipServiceServer)(nil),
	Methods: []grpc.MethodDesc{{
		MethodName: "CheckActiveMembership",
		Handler:    membershipServiceCheckHandler,
	}},
	Streams:  []grpc.StreamDesc{},
	Metadata: "membership-contract",
}

func membershipServiceCheckHandler(
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
		return srv.(MembershipServiceServer).CheckActiveMembership(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: MembershipServiceCheckActiveMemberPath}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(MembershipServiceServer).CheckActiveMembership(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

type membershipServer struct {
	db *gorm.DB
}

func NewMembershipServer(db *gorm.DB) MembershipServiceServer {
	return &membershipServer{db: db}
}

// CheckActiveMembership answers {"active": bool} for the requested membership.
// Only a row that belongs to the tenant, is active, and is not soft-deleted
// counts: another tenant's member, an inactive member, and an unknown id all
// answer false, never an error, so the caller cannot distinguish them. A
// database failure is an Internal error so the caller fails closed instead
// of guessing.
func (s *membershipServer) CheckActiveMembership(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
	tenantValue, err := structString(req, "tenant_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	parsedTenant, err := uuid.Parse(tenantValue)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "tenant_id must be a UUID")
	}
	memberValue, err := structString(req, "member_id")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	parsedMember, err := uuid.Parse(memberValue)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "member_id must be a UUID")
	}
	active, err := repository.IsActiveTenantMember(ctx, s.db, parsedTenant, parsedMember)
	if err != nil {
		return nil, status.Error(codes.Internal, "check membership: database error")
	}
	return structpb.NewStruct(map[string]interface{}{"active": active})
}
