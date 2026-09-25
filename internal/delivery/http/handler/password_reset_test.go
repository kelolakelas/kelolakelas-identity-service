package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
)

type resetAuthStub struct {
	requests     []string
	requestError error
	confirmError error
}

func (s *resetAuthStub) Register(context.Context, *domain.User, string) (*domain.User, error) {
	return nil, nil
}
func (s *resetAuthStub) RegisterInvitedUser(context.Context, string, string, string, string) (*domain.User, error) {
	return nil, nil
}
func (s *resetAuthStub) Login(context.Context, string, string) (string, *domain.User, uuid.UUID, error) {
	return "", nil, uuid.Nil, nil
}
func (s *resetAuthStub) RequestPasswordReset(_ context.Context, email string) error {
	s.requests = append(s.requests, email)
	return s.requestError
}
func (s *resetAuthStub) ConfirmPasswordReset(context.Context, string, string) error {
	return s.confirmError
}

func TestPasswordResetPublicResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &resetAuthStub{}
	h := NewAuthHandler(stub, nil)
	router := gin.New()
	router.POST("/request", h.RequestPasswordReset)
	router.POST("/confirm", h.ConfirmPasswordReset)
	call := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	known := call("/request", `{"email":"known@example.com"}`)
	unknown := call("/request", `{"email":"unknown@example.com"}`)
	stub.requestError = errors.New("storage unavailable")
	failed := call("/request", `{"email":"known@example.com"}`)
	if known.Code != 200 || known.Code != unknown.Code || known.Body.String() != unknown.Body.String() || known.Body.String() != failed.Body.String() {
		t.Fatalf("responses differ: %d %s / %d %s / %d %s", known.Code, known.Body, unknown.Code, unknown.Body, failed.Code, failed.Body)
	}
	for _, err := range []error{domain.ErrInvalidResetToken} {
		stub.confirmError = err
		w := call("/confirm", `{"token":"bad","password":"newpassword"}`)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "Invalid or expired reset token") {
			t.Fatalf("invalid token: %d %s", w.Code, w.Body)
		}
	}
}

type sessionStoreStub struct {
	boundary *time.Time
	err      error
}

func (s sessionStoreStub) Issue(context.Context, uuid.UUID, string, time.Time) error { return nil }
func (s sessionStoreStub) Consume(context.Context, string, string, time.Time) error  { return nil }
func (s sessionStoreStub) SessionValidAfter(context.Context, uuid.UUID) (*time.Time, error) {
	return s.boundary, s.err
}
func TestSessionCheckBoundaryAndOutage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens := jwt.NewJWTService("secret", time.Hour)
	id := uuid.New()
	token, err := tokens.GenerateToken(id, "x@example.com", uuid.Nil, uuid.Nil, uuid.Nil, true)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.ValidateToken(token)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		store sessionStoreStub
		want  int
	}{{sessionStoreStub{}, 204}, {sessionStoreStub{boundary: ptrTime(claims.IssuedAt.Time.Add(time.Second))}, 401}, {sessionStoreStub{boundary: ptrTime(claims.IssuedAt.Time)}, 204}, {sessionStoreStub{err: errors.New("db down")}, 503}}
	for _, tc := range cases {
		r := gin.New()
		r.GET("/check", SessionCheck(tokens, tc.store))
		request := httptest.NewRequest("GET", "/check", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d", w.Code, tc.want)
		}
	}
}
func ptrTime(t time.Time) *time.Time { return &t }
