package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

const sensitiveErrorMarker = "pq: duplicate key value violates unique constraint users_pkey"

// sensitiveErrAuthStub is a domain.AuthUsecase whose RegisterInvitedUser fails
// with an internal-looking database error, so the invited-registration 500
// branch can be driven from a handler-level test.
type sensitiveErrAuthStub struct {
	domain.AuthUsecase
	err error
}

func (s *sensitiveErrAuthStub) RegisterInvitedUser(context.Context, string, string, string, string) (*domain.User, error) {
	return nil, s.err
}

// sensitiveErrTenantStub is a domain.TenantUsecase whose RegisterTenant fails
// with an internal-looking database error, so the tenant-registration 500
// branch can be driven from a handler-level test.
type sensitiveErrTenantStub struct {
	domain.TenantUsecase
	err error
}

func (s *sensitiveErrTenantStub) RegisterTenant(context.Context, *domain.RegisterTenantRequest) (*domain.RegisterTenantResponse, error) {
	return nil, s.err
}

// capturedSlogRecords routes the default slog logger into a buffer the test
// can assert on. It returns a restore function so every test puts the process
// default logger back exactly as it found it.
func capturedSlogRecords(buffer *bytes.Buffer) (restore func()) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, nil)))
	return func() { slog.SetDefault(previous) }
}

// fiveHundredBody keeps the response envelope shape explicit: a sanitised 500
// must stay a {status, message, data} envelope.
type fiveHundredBody struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

func decodeFiveHundredBody(t *testing.T, recorder *httptest.ResponseRecorder) fiveHundredBody {
	t.Helper()
	var body fiveHundredBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, recorder.Body.String())
	}
	return body
}

// assertSanitised500 is the shared KEL-61 assertion for one endpoint: the
// default error branch answers 500 with the fixed generic message inside the
// unchanged envelope, never quotes the internal error, and the server log
// does carry the original error.
func assertSanitised500(t *testing.T, recorder *httptest.ResponseRecorder, logBuffer *bytes.Buffer, wantMessage string) {
	t.Helper()
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", recorder.Code, recorder.Body.String())
	}
	body := decodeFiveHundredBody(t, recorder)
	if body.Status != "error" {
		t.Fatalf("status field = %q, want error", body.Status)
	}
	if body.Data != nil {
		t.Fatalf("data = %v, want nil", body.Data)
	}
	if body.Message != wantMessage {
		t.Fatalf("message = %q, want the fixed generic message %q", body.Message, wantMessage)
	}
	if strings.Contains(recorder.Body.String(), sensitiveErrorMarker) {
		t.Fatal("500 body leaks the internal error marker")
	}
	if !strings.Contains(logBuffer.String(), sensitiveErrorMarker) {
		t.Fatalf("server log %q does not contain the original error", logBuffer.String())
	}
}

// TestRegisterTenant500KeepsInternalErrorOutOfResponse drives the default
// (unmapped) error branch of POST /api/v1/tenants/register with an
// internal-looking database error. KEL-61: the body must not quote the error,
// and the original error must reach the server log instead.
func TestRegisterTenant500KeepsInternalErrorOutOfResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuffer bytes.Buffer
	restore := capturedSlogRecords(&logBuffer)
	defer restore()

	stub := &sensitiveErrTenantStub{err: errors.New(sensitiveErrorMarker)}
	router := gin.New()
	router.POST("/tenants/register", NewAuthHandler(nil, stub).RegisterTenant)

	body := `{"email":"owner@example.com","password":"password","first_name":"First","last_name":"Last","tenant_name":"Tenant"}`
	request := httptest.NewRequest(http.MethodPost, "/tenants/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assertSanitised500(t, recorder, &logBuffer, "Failed to register tenant and user")
}

// TestCreateInvitation500KeepsInternalErrorOutOfResponse does the same for
// POST /api/v1/invitations: an unmapped usecase error yields the fixed
// generic 500, while the original error is only in the server log.
func TestCreateInvitation500KeepsInternalErrorOutOfResponse(t *testing.T) {
	var logBuffer bytes.Buffer
	restore := capturedSlogRecords(&logBuffer)
	defer restore()

	recorder := performCreateInvitation(&deliveryStatusInvitationUsecase{err: errors.New(sensitiveErrorMarker)})

	assertSanitised500(t, recorder, &logBuffer, "Failed to create invitation")
}

// TestRegisterInvitedUser500KeepsInternalErrorOutOfResponse covers POST
// /api/v1/invitations/register: same marker, same fixed generic 500, error
// only in the log.
func TestRegisterInvitedUser500KeepsInternalErrorOutOfResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logBuffer bytes.Buffer
	restore := capturedSlogRecords(&logBuffer)
	defer restore()

	stub := &sensitiveErrAuthStub{err: errors.New(sensitiveErrorMarker)}
	router := gin.New()
	router.POST("/invitations/register", NewInvitationHandler(nil, stub).RegisterInvitedUser)

	body := `{"token":"invitation-token","first_name":"First","last_name":"Last","password":"password"}`
	request := httptest.NewRequest(http.MethodPost, "/invitations/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assertSanitised500(t, recorder, &logBuffer, "Failed to register invited user")
}
