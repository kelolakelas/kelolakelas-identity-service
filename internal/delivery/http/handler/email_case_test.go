package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type emailCaseAuthSpy struct {
	domain.AuthUsecase
	registered, loggedIn, reset string
	registerErr                 error
}

func (s *emailCaseAuthSpy) Register(_ context.Context, user *domain.User, _ string) (*domain.User, error) {
	s.registered = user.Email
	if s.registerErr != nil {
		return nil, s.registerErr
	}
	return user, nil
}

func (s *emailCaseAuthSpy) Login(_ context.Context, email, _ string) (string, *domain.User, uuid.UUID, error) {
	s.loggedIn = email
	return "", nil, uuid.Nil, domain.ErrInvalidCredentials
}

func (s *emailCaseAuthSpy) RequestPasswordReset(_ context.Context, email string) error {
	s.reset = email
	return nil
}

func postJSON(t *testing.T, router *gin.Engine, path string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

// KEL-89: addresses typed with other letter case or surrounding spaces are
// the same account address, reach the usecase in canonical form, and a
// case-only duplicate is reported as 409.
func TestAuthHandlersNormalizeEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	spy := &emailCaseAuthSpy{}
	h := NewAuthHandler(spy, nil)
	router := gin.New()
	router.POST("/register", h.Register)
	router.POST("/login", h.Login)
	router.POST("/reset", h.RequestPasswordReset)

	if r := postJSON(t, router, "/register", map[string]any{"email": " New.User@Example.COM ", "password": "password", "first_name": "A", "last_name": "B"}); r.Code != http.StatusCreated {
		t.Fatalf("register status %d: %s", r.Code, r.Body.String())
	}
	if spy.registered != "new.user@example.com" {
		t.Fatalf("register got %q", spy.registered)
	}
	if r := postJSON(t, router, "/login", map[string]any{"email": "NEW.USER@example.com", "password": "password"}); r.Code != http.StatusUnauthorized {
		t.Fatalf("login status %d", r.Code)
	}
	if spy.loggedIn != "new.user@example.com" {
		t.Fatalf("login got %q", spy.loggedIn)
	}
	if r := postJSON(t, router, "/reset", map[string]any{"email": "\tNew.User@Example.com"}); r.Code != http.StatusOK {
		t.Fatalf("reset status %d", r.Code)
	}
	if spy.reset != "new.user@example.com" {
		t.Fatalf("reset got %q", spy.reset)
	}

	spy.registerErr = domain.ErrUserAlreadyExists
	if r := postJSON(t, router, "/register", map[string]any{"email": "NEW.USER@EXAMPLE.COM", "password": "password", "first_name": "A", "last_name": "B"}); r.Code != http.StatusConflict {
		t.Fatalf("case-variant duplicate status %d, want 409", r.Code)
	}
	for _, bad := range []any{"not-an-email", "", "   ", 42} {
		if r := postJSON(t, router, "/register", map[string]any{"email": bad, "password": "password", "first_name": "A", "last_name": "B"}); r.Code != http.StatusBadRequest {
			t.Fatalf("email %v status %d, want 400", bad, r.Code)
		}
	}
}
