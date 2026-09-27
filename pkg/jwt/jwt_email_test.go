package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// KEL-89: the email claim is the canonical account address even when a
// legacy row is stored mixed-case, so downstream services compare one form.
func TestGenerateTokenNormalizesEmailClaim(t *testing.T) {
	service := NewJWTService("secret", time.Hour)
	token, err := service.GenerateToken(uuid.New(), " Legacy.User@Example.COM ", uuid.Nil, uuid.Nil, uuid.Nil, true)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Email != "legacy.user@example.com" {
		t.Fatalf("email claim = %q", claims.Email)
	}
	platform, err := service.GeneratePlatformToken(uuid.New(), "Admin@Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	if claims, err = service.ValidateToken(platform); err != nil || claims.Email != "admin@example.com" {
		t.Fatalf("platform email claim = %v, %v", claims, err)
	}
}
