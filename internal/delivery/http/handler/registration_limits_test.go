package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type registrationAuthSpy struct {
	domain.AuthUsecase
	calls int
}

func (s *registrationAuthSpy) Register(_ context.Context, user *domain.User, _ string) (*domain.User, error) {
	s.calls++
	return user, nil
}
func (s *registrationAuthSpy) RegisterInvitedUser(_ context.Context, _, _, _, _ string) (*domain.User, error) {
	s.calls++
	return &domain.User{}, nil
}

type registrationTenantSpy struct {
	domain.TenantUsecase
	calls       int
	settingsErr error
}

func (s *registrationTenantSpy) RegisterTenant(_ context.Context, _ *domain.RegisterTenantRequest) (*domain.RegisterTenantResponse, error) {
	s.calls++
	return &domain.RegisterTenantResponse{}, nil
}
func (s *registrationTenantSpy) UpdateTenantSettings(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *domain.UpdateTenantSettingsRequest) (*domain.Tenant, error) {
	s.calls++
	return &domain.Tenant{}, s.settingsErr
}
func (s *registrationTenantSpy) UpdateTenantLocation(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *domain.UpdateTenantLocationRequest) (*domain.TenantLocation, error) {
	s.calls++
	return &domain.TenantLocation{}, nil
}

func TestRegistrationInputLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"register", "tenant", "invited"} {
		for _, tc := range []struct {
			name  string
			key   string
			value string
			want  int
		}{
			{"password 73 bytes", "password", strings.Repeat("é", 36) + "a", http.StatusBadRequest},
			{"password 72 bytes", "password", strings.Repeat("é", 36), http.StatusCreated},
			{"first name 256", "first_name", strings.Repeat("a", 256), http.StatusBadRequest},
			{"first name 255", "first_name", strings.Repeat("a", 255), http.StatusCreated},
			{"last name 256", "last_name", strings.Repeat("a", 256), http.StatusBadRequest},
			{"phone 51", "phone", strings.Repeat("1", 51), http.StatusBadRequest},
			{"phone 50", "phone", strings.Repeat("1", 50), http.StatusCreated},
		} {
			if endpoint == "invited" && tc.key == "phone" {
				continue
			}
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				auth, tenant := &registrationAuthSpy{}, &registrationTenantSpy{}
				router := gin.New()
				payload := map[string]any{"email": "valid@example.com", "password": "password", "first_name": "First", "last_name": "Last", "tenant_name": "My Tenant", "token": "invitation"}
				payload[tc.key] = tc.value
				var path string
				switch endpoint {
				case "register":
					path = "/register"
					router.POST(path, NewAuthHandler(auth, tenant).Register)
				case "tenant":
					path = "/tenant"
					router.POST(path, NewAuthHandler(auth, tenant).RegisterTenant)
				default:
					path = "/invited"
					router.POST(path, NewInvitationHandler(nil, auth).RegisterInvitedUser)
				}
				body, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != tc.want {
					t.Fatalf("status = %d, want %d: %s", response.Code, tc.want, response.Body.String())
				}
				calls := auth.calls + tenant.calls
				if tc.want == http.StatusBadRequest && calls != 0 {
					t.Fatalf("invalid input reached usecase %d times", calls)
				}
				if tc.want == http.StatusCreated && calls != 1 {
					t.Fatalf("valid input reached usecase %d times", calls)
				}
			})
		}
	}
}

func TestTenantRegistrationFieldLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		key, value string
		want       int
	}{
		{"tenant_name", strings.Repeat("a", 256), 400},
		{"tenant_name", strings.Repeat("a", 255), 201},
		{"tenant_phone", strings.Repeat("1", 51), 400},
		{"tenant_phone", strings.Repeat("1", 50), 201},
	} {
		t.Run(fmt.Sprintf("%s/%d", tc.key, len(tc.value)), func(t *testing.T) {
			spy := &registrationTenantSpy{}
			router := gin.New()
			router.POST("/tenant", NewAuthHandler(nil, spy).RegisterTenant)
			payload := map[string]any{"email": "valid@example.com", "password": "password", "first_name": "First", "last_name": "Last", "tenant_name": "Tenant"}
			payload[tc.key] = tc.value
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/tenant", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.want, response.Body.String())
			}
			if tc.want == 400 && spy.calls != 0 {
				t.Fatal("invalid input reached usecase")
			}
			if tc.want == 201 && spy.calls != 1 {
				t.Fatal("valid input did not reach usecase")
			}
		})
	}
}

func TestTenantSettingsNameConflictMapsTo409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	spy := &registrationTenantSpy{settingsErr: domain.ErrTenantNameAlreadyExists}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("tenant_id", uuid.New()); c.Next() })
	router.PATCH("/settings", NewTenantHandler(spy).UpdateSettings)
	request := httptest.NewRequest(http.MethodPatch, "/settings", strings.NewReader(`{"name":"Taken"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || spy.calls != 1 || !strings.Contains(response.Body.String(), `"status":"error"`) {
		t.Fatalf("code=%d calls=%d body=%s", response.Code, spy.calls, response.Body.String())
	}
}

func TestTenantSettingsAndLocationInputLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path, key, value string
		want             int
	}{
		{"/settings", "name", strings.Repeat("a", 256), 400},
		{"/settings", "name", strings.Repeat("a", 255), 200},
		{"/settings", "phone", strings.Repeat("1", 51), 400},
		{"/location", "google_place_id", strings.Repeat("x", 256), 400},
		{"/location", "google_place_id", strings.Repeat("x", 255), 200},
	} {
		t.Run(fmt.Sprintf("%s/%s/%d", tc.path, tc.key, len(tc.value)), func(t *testing.T) {
			spy := &registrationTenantSpy{}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("tenant_id", uuid.New()); c.Next() })
			handler := NewTenantHandler(spy)
			router.PATCH("/settings", handler.UpdateSettings)
			router.PUT("/location", handler.UpdateLocation)
			payload := map[string]any{"name": "Tenant", "address": "Address"}
			payload[tc.key] = tc.value
			body, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			method := http.MethodPatch
			if tc.path == "/location" {
				method = http.MethodPut
			}
			request := httptest.NewRequest(method, tc.path, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.want, response.Body.String())
			}
			if tc.want == 400 && spy.calls != 0 {
				t.Fatal("invalid input reached usecase")
			}
			if tc.want == 200 && spy.calls != 1 {
				t.Fatal("valid input did not reach usecase")
			}
		})
	}
}
