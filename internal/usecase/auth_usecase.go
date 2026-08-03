package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/database"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/hash"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type authUsecase struct {
	userRepo     domain.UserRepository
	jwtService   *jwt.JWTService
	redisService *database.RedisService
}

func NewAuthUsecase(userRepo domain.UserRepository, jwtService *jwt.JWTService, redisService *database.RedisService) *authUsecase {
	return &authUsecase{
		userRepo:     userRepo,
		jwtService:   jwtService,
		redisService: redisService,
	}
}

func (u *authUsecase) Register(ctx context.Context, user *domain.User, password string) (*domain.User, error) {
	// Check if user already exists
	existing, err := u.userRepo.GetByEmail(ctx, user.Email)
	if err == nil && existing != nil {
		return nil, domain.ErrUserAlreadyExists
	}

	// Hash password
	hashedPassword, err := hash.HashPassword(password)
	if err != nil {
		return nil, err
	}
	user.PasswordHash = hashedPassword

	// Generate UUID if not set
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	// Save user
	if err := u.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (u *authUsecase) RegisterInvitedUser(ctx context.Context, token, firstName, lastName, password string) (*domain.User, error) {
	return u.userRepo.RegisterInvitedUserTx(ctx, token, firstName, lastName, password)
}

func (u *authUsecase) Login(ctx context.Context, email, password string) (string, *domain.User, uuid.UUID, error) {
	// Get user
	user, err := u.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return "", nil, uuid.Nil, domain.ErrInvalidCredentials
		}
		return "", nil, uuid.Nil, err
	}

	// Verify password
	if !hash.CheckPasswordHash(password, user.PasswordHash) {
		return "", nil, uuid.Nil, domain.ErrInvalidCredentials
	}

	// Find active tenant member
	var tenantID, roleID, memberID uuid.UUID
	member, err := u.userRepo.GetTenantMemberByUserID(ctx, user.ID)
	if err == nil && member != nil {
		tenantID = member.TenantID
		roleID = member.RoleID
		memberID = member.ID
	}

	// Generate token
	token, err := u.jwtService.GenerateToken(user.ID, user.Email, tenantID, roleID, memberID, user.IsParent)
	if err != nil {
		return "", nil, uuid.Nil, err
	}

	// Save permissions to Redis
	if roleID != uuid.Nil && u.redisService != nil {
		permissions, err := u.userRepo.GetPermissionsByRoleId(ctx, roleID)
		if err == nil {
			_ = u.redisService.SaveRolePermissions(ctx, tenantID, roleID, permissions, 24*time.Hour)
		}
	}

	return token, user, tenantID, nil
}
