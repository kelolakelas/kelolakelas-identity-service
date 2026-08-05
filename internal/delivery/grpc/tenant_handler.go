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

func (s *TenantServiceServer) GetTenantPublicInfo(ctx context.Context, req *pb.TenantPublicInfoRequest) (*pb.TenantPublicInfoResponse, error) {
	ids := make([]uuid.UUID, 0, len(req.GetTenantIds()))
	for _, value := range req.GetTenantIds() {
		id, err := uuid.Parse(value)
		if err == nil {
			ids = append(ids, id)
		}
	}
	var tenants []domain.Tenant
	if len(ids) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&tenants).Error; err != nil {
			return nil, err
		}
	}
	response := &pb.TenantPublicInfoResponse{Tenants: make([]*pb.TenantPublicInfo, 0, len(tenants))}
	for _, tenant := range tenants {
		info := &pb.TenantPublicInfo{TenantId: tenant.ID.String(), Name: tenant.Name, IsActive: tenant.Status == "active"}
		if tenant.AddressFormatted != nil {
			info.AddressFormatted = *tenant.AddressFormatted
		}
		if tenant.Latitude != nil && tenant.Longitude != nil {
			info.Latitude, info.Longitude, info.HasLocation = *tenant.Latitude, *tenant.Longitude, true
		}
		response.Tenants = append(response.Tenants, info)
	}
	return response, nil
}
