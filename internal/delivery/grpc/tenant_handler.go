package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	pb "github.com/kelolakelas/kelolakelas-identity-service/pkg/proto/tenant"
)

type TenantServiceServer struct {
	pb.UnimplementedTenantServiceServer
	db *gorm.DB
}

func NewTenantServiceServer(db *gorm.DB) *TenantServiceServer {
	return &TenantServiceServer{db: db}
}

func (s *TenantServiceServer) ValidateTenantStatus(ctx context.Context, req *pb.ValidateTenantRequest) (*pb.ValidateTenantResponse, error) {
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return &pb.ValidateTenantResponse{
			IsActive: false,
			Message:  "Invalid tenant ID format",
		}, nil
	}

	var tenant domain.Tenant
	if err := s.db.WithContext(ctx).First(&tenant, "id = ?", tenantID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.ValidateTenantResponse{
				IsActive: false,
				Message:  "Tenant not found",
			}, nil
		}
		return &pb.ValidateTenantResponse{
			IsActive: false,
			Message:  "Database error: " + err.Error(),
		}, nil
	}

	if tenant.Status != "active" {
		return &pb.ValidateTenantResponse{
			IsActive: false,
			Message:  "Tenant status is " + tenant.Status,
		}, nil
	}

	return &pb.ValidateTenantResponse{
		IsActive: true,
		Message:  "Tenant is active",
	}, nil
}
