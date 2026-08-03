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
	UserID   uuid.UUID `json:"user_id"`
	Email    string    `json:"email"`
	TenantID uuid.UUID `json:"tenant_id,omitempty"`
	RoleID   uuid.UUID `json:"role_id,omitempty"`
	MemberID uuid.UUID `json:"member_id,omitempty"`
	IsParent bool      `json:"is_parent,omitempty"`
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
	claims := Claims{
		UserID:   userID,
		Email:    email,
		TenantID: tenantID,
		RoleID:   roleID,
		MemberID: memberID,
		IsParent: isParent,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.tokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

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
