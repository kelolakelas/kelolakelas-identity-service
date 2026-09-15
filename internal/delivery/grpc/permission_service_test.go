package grpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestCheckPermissionValidatesContractBeforeDatabaseAccess(t *testing.T) {
	server := &TenantServiceServer{}
	cases := []struct {
		name string
		req  *structpb.Struct
		want string
	}{
		{name: "missing role", req: &structpb.Struct{}, want: "role_id is required"},
		{name: "missing permission", req: mustStruct(map[string]interface{}{"role_id": "role"}), want: "permission is required"},
		{name: "invalid role UUID", req: mustStruct(map[string]interface{}{"role_id": "role", "permission": "class:update"}), want: "role_id must be a UUID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server.CheckPermission(context.Background(), tc.req)
			if status.Code(err) != codes.InvalidArgument || err.Error() == "" || !contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want InvalidArgument containing %q", err, tc.want)
			}
		})
	}
}

func mustStruct(values map[string]interface{}) *structpb.Struct {
	result, err := structpb.NewStruct(values)
	if err != nil {
		panic(err)
	}
	return result
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
