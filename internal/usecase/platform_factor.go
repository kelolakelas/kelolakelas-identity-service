package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

func factorCode(secret []byte, counter int64) string {
	mac := hmac.New(sha1.New, secret)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))
	mac.Write(buf[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	number := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", number%1000000)
}
func checkFactor(secret []byte, code string, now time.Time) (bool, error) {
	if len(code) != 6 {
		return false, nil
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false, nil
	}
	for offset := -1; offset <= 1; offset++ {
		if hmac.Equal([]byte(factorCode(secret, now.Unix()/30+int64(offset))), []byte(code)) {
			return true, nil
		}
	}
	return false, nil
}
func (a *PlatformAuth) WithFactor(store domain.PlatformFactorStore) *PlatformAuth {
	a.factor = store
	return a
}
func (a *PlatformAuth) StartFactor(ctx context.Context, pending string, purpose string) (string, string, error) {
	claims, err := a.pending(ctx, pending)
	if err != nil {
		return "", "", err
	}
	if a.factor == nil {
		return "", "", ErrPlatformUnavailable
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return "", "", err
	}
	var secret []byte
	if purpose == "enroll" {
		secret = make([]byte, 20)
		if _, err = rand.Read(secret); err != nil {
			return "", "", err
		}
	}
	if _, err = a.factor.Begin(ctx, claims.UserID, purpose, token, secret, time.Now().Add(5*time.Minute)); err != nil {
		return "", "", ErrPlatformUnavailable
	}
	return hex.EncodeToString(token), strings.TrimRight(base32.StdEncoding.EncodeToString(secret), "="), nil
}
func (a *PlatformAuth) FinishFactor(ctx context.Context, pending, challenge, code, purpose string) (string, error) {
	claims, err := a.pending(ctx, pending)
	if err != nil {
		return "", err
	}
	if a.factor == nil {
		return "", ErrPlatformUnavailable
	}
	token, err := hex.DecodeString(challenge)
	if err != nil || len(token) != 32 {
		return "", ErrPlatformForbidden
	}
	accepted, version, err := a.factor.Consume(ctx, claims.UserID, purpose, token, time.Now(), func(secret []byte) (bool, error) { return checkFactor(secret, code, time.Now()) })
	if err != nil {
		return "", ErrPlatformUnavailable
	}
	if !accepted {
		return "", ErrPlatformForbidden
	}
	if err := a.CheckVersion(ctx, claims.UserID, version); err != nil {
		return "", err
	}
	user, err := a.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return "", err
	}
	return a.tokens.GenerateVerifiedPlatformToken(user.ID, user.Email, nil, version)
}
func (a *PlatformAuth) pending(ctx context.Context, token string) (*jwt.Claims, error) {
	claims, err := a.tokens.ValidateToken(token)
	if err != nil || !claims.PlatformPending || claims.IsPlatformAdmin || claims.UserID == uuid.Nil {
		return nil, ErrPlatformForbidden
	}
	if err := a.Check(ctx, claims.UserID); err != nil {
		return nil, err
	}
	if a.factor == nil {
		return nil, ErrPlatformUnavailable
	}
	version, err := a.factor.AssignmentVersion(ctx, claims.UserID)
	if err != nil {
		return nil, ErrPlatformUnavailable
	}
	if version != claims.PlatformPendingVersion {
		return nil, ErrPlatformForbidden
	}
	return claims, nil
}
