package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/database"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type tenantUsecase struct {
	userRepo     domain.UserRepository
	tenantRepo   repository.TenantRepository
	jwtService   *jwt.JWTService
	redisService *database.RedisService
}

func NewTenantUsecase(userRepo domain.UserRepository, tenantRepo repository.TenantRepository, jwtService *jwt.JWTService, redisService *database.RedisService) domain.TenantUsecase {
	return &tenantUsecase{
		userRepo:     userRepo,
		tenantRepo:   tenantRepo,
		jwtService:   jwtService,
		redisService: redisService,
	}
}

func (u *tenantUsecase) RegisterTenant(ctx context.Context, req *domain.RegisterTenantRequest) (*domain.RegisterTenantResponse, error) {
	// Check if tenant name already exists
	nameExists, err := u.tenantRepo.IsNameExists(ctx, req.TenantName)
	if err != nil {
		return nil, err
	}
	if nameExists {
		return nil, domain.ErrTenantNameAlreadyExists
	}

	// Check if user already exists
	existing, err := u.userRepo.GetByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		return nil, domain.ErrUserAlreadyExists
	}

	// Hash password
	hashedPassword, err := hash.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		ID:           uuid.New(),
		Email:        req.Email,
		PasswordHash: hashedPassword,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Phone:        req.Phone,
		IsParent:     false,
	}

	tenant := &domain.Tenant{
		ID:      uuid.New(),
		Name:    req.TenantName,
		Phone:   req.TenantPhone,
		Address: req.TenantAddress,
		Status:  "active",
	}

	// Save using GORM transaction
	member, err := u.userRepo.RegisterTenantTx(ctx, user, tenant)
	if err != nil {
		return nil, err
	}

	// Generate Token
	token, err := u.jwtService.GenerateToken(user.ID, user.Email, tenant.ID, member.RoleID)
	if err != nil {
		return nil, err
	}

	// Save permissions to Redis
	if u.redisService != nil {
		permissions, err := u.userRepo.GetPermissionsByRoleId(ctx, member.RoleID)
		if err == nil {
			_ = u.redisService.SaveRolePermissions(ctx, tenant.ID, member.RoleID, permissions, 24*time.Hour)
		}
	}

	return &domain.RegisterTenantResponse{
		Token:  token,
		User:   *user,
		Tenant: *tenant,
	}, nil
}
