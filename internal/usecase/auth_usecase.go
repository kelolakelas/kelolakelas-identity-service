package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
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
	resets       domain.PasswordResetRepository
	resetEmail   interface{ SendPasswordResetEmail(string, string) error }
	resetTTL     time.Duration
}

func NewAuthUsecase(userRepo domain.UserRepository, jwtService *jwt.JWTService, redisService *database.RedisService) *authUsecase {
	return &authUsecase{
		userRepo:     userRepo,
		jwtService:   jwtService,
		redisService: redisService,
	}
}

func (u *authUsecase) WithPasswordReset(store domain.PasswordResetRepository, sender interface{ SendPasswordResetEmail(string, string) error }, ttl time.Duration) *authUsecase {
	u.resets, u.resetEmail, u.resetTTL = store, sender, ttl
	return u
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

func resetTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (u *authUsecase) RequestPasswordReset(ctx context.Context, email string) error {
	user, err := u.userRepo.GetByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	token := hex.EncodeToString(secret)
	if err := u.resets.Issue(ctx, user.ID, resetTokenHash(token), time.Now().Add(u.resetTTL)); err != nil {
		return err
	}
	if err := u.resetEmail.SendPasswordResetEmail(user.Email, token); err != nil {
		slog.ErrorContext(ctx, "password reset email delivery failed", "user_id", user.ID, "error", err)
	}
	return nil
}

func (u *authUsecase) ConfirmPasswordReset(ctx context.Context, token, password string) error {
	if len(token) != 64 {
		return domain.ErrInvalidResetToken
	}
	if _, err := hex.DecodeString(token); err != nil {
		return domain.ErrInvalidResetToken
	}
	passwordHash, err := hash.HashPassword(password)
	if err != nil {
		return err
	}
	return u.resets.Consume(ctx, resetTokenHash(token), passwordHash, time.Now())
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

	// Read the committed boundary before issuing any new session. A failed read
	// must not issue a token that the gateway will immediately reject.
	var validAfter *time.Time
	if u.resets != nil {
		validAfter, err = u.resets.SessionValidAfter(ctx, user.ID)
		if err != nil {
			return "", nil, uuid.Nil, err
		}
	}
	token, err := u.jwtService.GenerateTokenAfter(user.ID, user.Email, tenantID, roleID, memberID, user.IsParent, validAfter)
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
