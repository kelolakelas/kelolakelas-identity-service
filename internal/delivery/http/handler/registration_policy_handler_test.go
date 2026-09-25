package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// registrationPolicyHandlerStub is a configurable domain.RegistrationPolicy
// for the KEL-97 handler tests.
type registrationPolicyHandlerStub struct {
	evaluation   domain.RegistrationPolicyEvaluated
	evaluateErr  error
	closeErr     error
	openErr      error
	closeCalls   int
	openCalls    int
	lastOperator uuid.UUID
}

func (s *registrationPolicyHandlerStub) Evaluate(context.Context) (domain.RegistrationPolicyEvaluated, error) {
	return s.evaluation, s.evaluateErr
}
func (s *registrationPolicyHandlerStub) Close(_ context.Context, operator uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	s.closeCalls++
	s.lastOperator = operator
	if s.closeErr != nil {
		return domain.RegistrationPolicyEvaluated{}, s.closeErr
	}
	return domain.RegistrationPolicyEvaluated{Open: true, AppliedVersion: 1, DesiredVersion: 2}, nil
}
func (s *registrationPolicyHandlerStub) Open(_ context.Context, operator uuid.UUID) (domain.RegistrationPolicyEvaluated, error) {
	s.openCalls++
	s.lastOperator = operator
	if s.openErr != nil {
		return domain.RegistrationPolicyEvaluated{}, s.openErr
	}
	return domain.RegistrationPolicyEvaluated{Open: false, AppliedVersion: 2, DesiredVersion: 3}, nil
}

func registrationPolicyRouter(policy domain.RegistrationPolicy) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	addOperator := func(c *gin.Context) { c.Set("user_id", uuid.New()); c.Next() }
	configurationHandler := NewConfigurationHandler(nil, policy)
	router.GET("/platform/registration-policy", addOperator, configurationHandler.RegistrationPolicy)
	router.POST("/platform/registration-policy/close", addOperator, configurationHandler.CloseRegistration)
	router.POST("/platform/registration-policy/open", addOperator, configurationHandler.OpenRegistration)
	return router
}

func TestRegistrationPolicyEndpointsReadAndMutate(t *testing.T) {
	stub := &registrationPolicyHandlerStub{evaluation: domain.RegistrationPolicyEvaluated{
		Application: domain.TenantOnboardingApplication, Key: domain.TenantRegistrationOpenKey,
		Environment: domain.TenantRegistrationEnvironment, Open: true, AppliedVersion: 1, DesiredVersion: 1,
	}}
	router := registrationPolicyRouter(stub)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/registration-policy", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"applied_version":1`) || !strings.Contains(response.Body.String(), `"desired_version":1`) {
		t.Fatalf("GET did not report desired and applied versions: %s", response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/platform/registration-policy/close", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("close status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.closeCalls != 1 {
		t.Fatalf("close calls=%d", stub.closeCalls)
	}
	for _, field := range []string{`"open":true`, `"applied_version":1`, `"desired_version":2`} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("pending close response missing %s: %s", field, response.Body.String())
		}
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/platform/registration-policy/open", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("open status=%d body=%s", response.Code, response.Body.String())
	}
	if stub.openCalls != 1 {
		t.Fatalf("open calls=%d", stub.openCalls)
	}
	for _, field := range []string{`"open":false`, `"applied_version":2`, `"desired_version":3`} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("pending open response missing %s: %s", field, response.Body.String())
		}
	}
	if stub.lastOperator == uuid.Nil {
		t.Fatal("mutation did not receive the platform operator identity")
	}
}

func TestRegistrationPolicyEndpointsMapErrors(t *testing.T) {
	tests := []struct {
		name       string
		policy     *registrationPolicyHandlerStub
		path       string
		wantStatus int
	}{
		{name: "evaluate outage", policy: &registrationPolicyHandlerStub{evaluateErr: errors.New("store down")}, path: "/platform/registration-policy", wantStatus: http.StatusInternalServerError},
		{name: "close conflict", policy: &registrationPolicyHandlerStub{closeErr: domain.ErrConfigurationVersionConflict}, path: "/platform/registration-policy/close", wantStatus: http.StatusConflict},
		{name: "close outage", policy: &registrationPolicyHandlerStub{closeErr: errors.New("store down")}, path: "/platform/registration-policy/close", wantStatus: http.StatusInternalServerError},
		{name: "open conflict", policy: &registrationPolicyHandlerStub{openErr: domain.ErrConfigurationVersionConflict}, path: "/platform/registration-policy/open", wantStatus: http.StatusConflict},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router := registrationPolicyRouter(tc.policy)
			method := http.MethodGet
			if tc.path != "/platform/registration-policy" {
				method = http.MethodPost
			}
			router.ServeHTTP(response, httptest.NewRequest(method, tc.path, nil))
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}

// registrationAuthSpy records whether the usecase was reached and lets tests
// choose the error it returns.
type registrationClosedAuthSpy struct {
	domain.AuthUsecase
	err   error
	calls int
}

func (s *registrationClosedAuthSpy) Register(_ context.Context, user *domain.User, _ string) (*domain.User, error) {
	s.calls++
	return user, nil
}
func (s *registrationClosedAuthSpy) RegisterInvitedUser(_ context.Context, _, _, _, _ string) (*domain.User, error) {
	s.calls++
	return &domain.User{}, nil
}

type registrationClosedTenantSpy struct {
	domain.TenantUsecase
	err   error
	calls int
}

func (s *registrationClosedTenantSpy) RegisterTenant(_ context.Context, _ *domain.RegisterTenantRequest) (*domain.RegisterTenantResponse, error) {
	s.calls++
	return &domain.RegisterTenantResponse{}, s.err
}

func tenantRegisterRouter(tenant domain.TenantUsecase) *gin.Engine {
	router := gin.New()
	router.POST("/api/v1/tenants/register", NewAuthHandler(nil, tenant).RegisterTenant)
	return router
}

func validTenantRegistrationBody() string {
	body, err := json.Marshal(map[string]any{
		"email": "owner@example.com", "password": "password", "first_name": "First",
		"last_name": "Last", "tenant_name": "Fresh Tenant",
	})
	if err != nil {
		tFatal(err)
	}
	return string(body)
}

func tFatal(err error) { panic(err) }

func TestRegisterTenantHandlerMapsClosedRegistrationStably(t *testing.T) {
	spy := &registrationClosedTenantSpy{err: domain.ErrRegistrationClosed}
	router := tenantRegisterRouter(spy)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/register", strings.NewReader(validTenantRegistrationBody()))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if decoded.Status != "error" || decoded.Message != domain.RegistrationClosedMessage {
		t.Fatalf("unstable rejection: %+v", decoded)
	}
	if spy.calls != 1 {
		t.Fatalf("usecase calls=%d", spy.calls)
	}
}

func TestRegisterTenantHandlerValidationStillPrecedesPolicyRejection(t *testing.T) {
	spy := &registrationClosedTenantSpy{err: domain.ErrRegistrationClosed}
	router := tenantRegisterRouter(spy)

	// An oversized field is rejected with 400 before the policy is consulted
	// and before the usecase rejects with the closed-registration error.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/register",
		strings.NewReader(`{"email":"owner@example.com","password":"password","first_name":"`+strings.Repeat("a", 256)+`","last_name":"Last","tenant_name":"Fresh Tenant"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if spy.calls != 0 {
		t.Fatalf("invalid input reached the usecase %d times", spy.calls)
	}
}

func TestRegisterTenantHandlerKeepsOtherDomainErrorsUnchanged(t *testing.T) {
	spy := &registrationClosedTenantSpy{err: domain.ErrTenantNameAlreadyExists}
	router := tenantRegisterRouter(spy)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/register", strings.NewReader(validTenantRegistrationBody()))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), domain.RegistrationClosedMessage) {
		t.Fatalf("name conflict was masked as registration closed: %s", response.Body.String())
	}
}
