package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type testPlatformAdmins struct {
	active bool
	err    error
}

func (s *testPlatformAdmins) IsActive(context.Context, uuid.UUID) (bool, error) {
	return s.active, s.err
}

type testPlatformFactor struct{ domain.PlatformFactorStore }

func (testPlatformFactor) Version(context.Context, uuid.UUID) (int64, error) { return 1, nil }

type testPlatformUsers struct{ domain.UserRepository }

func (testPlatformUsers) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	return &domain.User{ID: id}, nil
}

func TestPlatformAdminServiceLiveAssignment(t *testing.T) {
	admins := &testPlatformAdmins{active: true}
	server := NewPlatformAdminServer(usecase.NewPlatformAuth(testPlatformUsers{}, admins, nil).WithFactor(testPlatformFactor{}))
	// CheckActive rejects missing factor version, even if the assignment exists.
	req, _ := structpb.NewStruct(map[string]interface{}{"user_id": uuid.NewString(), "platform_factor_version": float64(0)})
	resp, err := server.CheckActive(context.Background(), req)
	if err != nil || resp.GetFields()["allowed"].GetBoolValue() {
		t.Fatalf("unverified principal resp=%v err=%v", resp, err)
	}
	req, _ = structpb.NewStruct(map[string]interface{}{"user_id": uuid.NewString(), "platform_factor_version": float64(1)})
	resp, err = server.CheckActive(context.Background(), req)
	if err != nil || !resp.GetFields()["allowed"].GetBoolValue() {
		t.Fatalf("active resp=%v err=%v", resp, err)
	}
	admins.active = false
	req, _ = structpb.NewStruct(map[string]interface{}{"user_id": uuid.NewString(), "platform_factor_version": float64(1)})
	resp, err = server.CheckActive(context.Background(), req)
	if err != nil || resp.GetFields()["allowed"].GetBoolValue() {
		t.Fatalf("revoked resp=%v err=%v", resp, err)
	}
	admins.err = errors.New("db down")
	_, err = server.CheckActive(context.Background(), req)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("outage err=%v", err)
	}
}
