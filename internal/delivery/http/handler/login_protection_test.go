package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type loginAuthStub struct {
	domain.AuthUsecase
	err error
}

func (s loginAuthStub) Login(context.Context, string, string) (string, *domain.User, uuid.UUID, error) {
	return "", nil, uuid.Nil, s.err
}

func TestLoginCredentialFailuresShareResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var baseline string
	for _, tc := range []struct {
		name, email string
		err         error
	}{
		{"unknown email", "unknown@example.com", domain.ErrInvalidCredentials},
		{"wrong password", "known@example.com", domain.ErrInvalidCredentials},
		{"locked account", "locked@example.com", domain.ErrInvalidCredentials},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{"email": tc.email, "password": "wrong"})
			router := gin.New()
			router.POST("/login", NewAuthHandler(loginAuthStub{err: tc.err}, nil).Login)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(payload)))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d", response.Code)
			}
			if baseline == "" {
				baseline = response.Body.String()
			}
			if response.Body.String() != baseline {
				t.Fatalf("response differs: %q vs %q", response.Body.String(), baseline)
			}
		})
	}
	router := gin.New()
	router.POST("/login", NewAuthHandler(loginAuthStub{err: errors.New("store unavailable")}, nil).Login)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{"email":"known@example.com","password":"right"}`)))
	if response.Code != http.StatusInternalServerError || bytes.Contains(response.Body.Bytes(), []byte("store unavailable")) {
		t.Fatalf("store error exposed: %d %s", response.Code, response.Body.String())
	}
}
