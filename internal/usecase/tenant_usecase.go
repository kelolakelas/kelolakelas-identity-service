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
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/maps"
)

type tenantUsecase struct {
	userRepo     domain.UserRepository
	tenantRepo   repository.TenantRepository
	jwtService   *jwt.JWTService
	redisService *database.RedisService
	mapsClient   maps.MapsClient
}

func NewTenantUsecase(userRepo domain.UserRepository, tenantRepo repository.TenantRepository, jwtService *jwt.JWTService, redisService *database.RedisService, mapsClient maps.MapsClient) domain.TenantUsecase {
	return &tenantUsecase{
		userRepo:     userRepo,
		tenantRepo:   tenantRepo,
		jwtService:   jwtService,
		redisService: redisService,
		mapsClient:   mapsClient,
	}
}

func (u *tenantUsecase) GetTenantLocation(ctx context.Context, id uuid.UUID) (*domain.TenantLocation, error) {
	tenant, err := u.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &domain.TenantLocation{Address: valueOrEmpty(tenant.Address), AddressFormatted: tenant.AddressFormatted, Latitude: tenant.Latitude, Longitude: tenant.Longitude, GooglePlaceID: tenant.GooglePlaceID, LocationAccuracyMeters: tenant.LocationAccuracy, LocationUpdatedAt: tenant.LocationUpdatedAt}, nil
}

func (u *tenantUsecase) UpdateTenantLocation(ctx context.Context, id uuid.UUID, req *domain.UpdateTenantLocationRequest) (*domain.TenantLocation, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	tenant, err := u.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	formatted := req.Address
	latitude, longitude := req.Latitude, req.Longitude
	placeID := req.GooglePlaceID
	if latitude == nil && u.mapsClient != nil {
		geocoded, geoErr := u.mapsClient.Geocode(ctx, req.Address)
		if geoErr != nil {
			return nil, geoErr
		}
		latitude, longitude = &geocoded.Latitude, &geocoded.Longitude
		formatted, placeID = geocoded.FormattedAddress, stringPtr(geocoded.PlaceID)
	}
	now := time.Now().UTC()
	tenant.Address = &req.Address
	tenant.AddressFormatted = &formatted
	tenant.Latitude, tenant.Longitude = latitude, longitude
	tenant.GooglePlaceID = placeID
	tenant.LocationUpdatedAt = &now
	if err := u.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	return u.GetTenantLocation(ctx, id)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
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
	token, err := u.jwtService.GenerateToken(user.ID, user.Email, tenant.ID, member.RoleID, member.ID, false)
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
		Token: token, User: *user, Tenant: *tenant, TenantID: tenant.ID,
	}, nil
}

func (u *tenantUsecase) GetTenantByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return u.tenantRepo.GetByID(ctx, id)
}

func (u *tenantUsecase) UpdateTenantSettings(ctx context.Context, id uuid.UUID, req *domain.UpdateTenantSettingsRequest) (*domain.Tenant, error) {
	tenant, err := u.tenantRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	addressChanged := valueOrEmpty(tenant.Address) != valueOrEmpty(req.Address)
	tenant.Name = req.Name
	tenant.Phone = req.Phone
	tenant.Address = req.Address
	tenant.About = req.About
	if addressChanged {
		now := time.Now().UTC()
		tenant.LocationUpdatedAt = &now
	}
	if err := u.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, err
	}
	return tenant, nil
}
