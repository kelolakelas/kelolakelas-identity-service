package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

type Claims struct {
	UserID                 uuid.UUID `json:"user_id"`
	Email                  string    `json:"email"`
	TenantID               uuid.UUID `json:"tenant_id,omitempty"`
	RoleID                 uuid.UUID `json:"role_id,omitempty"`
	MemberID               uuid.UUID `json:"member_id,omitempty"`
	IsParent               bool      `json:"is_parent,omitempty"`
	IsPlatformAdmin        bool      `json:"is_platform_admin,omitempty"`
	PlatformFactorVersion  int64     `json:"platform_factor_version,omitempty"`
	PlatformPending        bool      `json:"platform_pending,omitempty"`
	PlatformPendingVersion int64     `json:"platform_pending_version,omitempty"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secretKey     []byte
	tokenDuration time.Duration
}

func NewJWTService(secretKey string, duration time.Duration) *JWTService {
	return &JWTService{
		secretKey:     []byte(secretKey),
		tokenDuration: duration,
	}
}

func (s *JWTService) GenerateToken(userID uuid.UUID, email string, tenantID, roleID, memberID uuid.UUID, isParent bool) (string, error) {
	return s.GenerateTokenAfter(userID, email, tenantID, roleID, memberID, isParent, nil)
}

func (s *JWTService) GenerateTokenAfter(userID uuid.UUID, email string, tenantID, roleID, memberID uuid.UUID, isParent bool, validAfter *time.Time) (string, error) {
	issued := time.Now()
	if validAfter != nil && issued.Before(*validAfter) {
		issued = *validAfter
	}
	claims := Claims{
		UserID:   userID,
		Email:    email,
		TenantID: tenantID,
		RoleID:   roleID,
		MemberID: memberID,
		IsParent: isParent,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(issued.Add(s.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(issued),
		},
	}

	return s.sign(claims)
}

// GeneratePlatformToken issues a tenantless platform principal. Ordinary tenant
// tokens cannot gain this claim through public registration or tenant roles.
func (s *JWTService) GeneratePlatformToken(userID uuid.UUID, email string) (string, error) {
	return s.GeneratePlatformTokenAfter(userID, email, nil)
}

func (s *JWTService) GeneratePlatformTokenAfter(userID uuid.UUID, email string, validAfter *time.Time) (string, error) {
	return s.GenerateVerifiedPlatformToken(userID, email, validAfter, 0)
}

func (s *JWTService) GeneratePendingPlatformToken(userID uuid.UUID, email string, version int64) (string, error) {
	return s.sign(Claims{UserID: userID, Email: email, PlatformPending: true, PlatformPendingVersion: version, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)), IssuedAt: jwt.NewNumericDate(time.Now())}})
}

func (s *JWTService) GenerateVerifiedPlatformToken(userID uuid.UUID, email string, validAfter *time.Time, version int64) (string, error) {
	issued := time.Now()
	if validAfter != nil && issued.Before(*validAfter) {
		issued = *validAfter
	}
	return s.sign(Claims{
		UserID: userID, Email: email, IsPlatformAdmin: true, PlatformFactorVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(issued.Add(s.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(issued),
		},
	})
}

func (s *JWTService) sign(claims Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secretKey)
}

func (s *JWTService) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
