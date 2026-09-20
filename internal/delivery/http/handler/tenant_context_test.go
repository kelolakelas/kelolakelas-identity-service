package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TestTenantIDFromContextSeparatesMissingFromInvalid covers the claim shapes
// that AuthMiddleware can produce so the three outcomes (accept, missing,
// invalid) stay distinguishable.
func TestTenantIDFromContextSeparatesMissingFromInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenant := uuid.New()

	cases := []struct {
		name      string
		stored    any
		set       bool
		wantID    uuid.UUID
		wantError error
	}{
		{name: "uuid claim is accepted", stored: tenant, set: true, wantID: tenant},
		{name: "nil uuid claim is missing", stored: uuid.Nil, set: true, wantError: errTenantContextMissing},
		{name: "absent claim is missing", set: false, wantError: errTenantContextMissing},
		{name: "empty string claim is missing", stored: "", set: true, wantError: errTenantContextMissing},
		{name: "nil string claim is missing", stored: uuid.Nil.String(), set: true, wantError: errTenantContextMissing},
		{name: "valid string claim is parsed", stored: tenant.String(), set: true, wantID: tenant},
		{name: "unparsable string claim is invalid", stored: "not-a-uuid", set: true, wantError: errTenantContextInvalid},
		{name: "unexpected claim type is invalid", stored: 42, set: true, wantError: errTenantContextInvalid},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			if testCase.set {
				context.Set("tenant_id", testCase.stored)
			}

			id, err := tenantIDFromContext(context)
			if err != testCase.wantError {
				t.Fatalf("err=%v, want %v", err, testCase.wantError)
			}
			if err == nil && id != testCase.wantID {
				t.Fatalf("id=%s, want %s", id, testCase.wantID)
			}
			if err != nil && id != uuid.Nil {
				t.Fatalf("id=%s, want uuid.Nil on failure", id)
			}
		})
	}
}

// TestTenantIDFromContextNeverReadsHeaders pins the core security property at
// the helper level: the presence of X-Tenant-ID cannot influence the result.
func TestTenantIDFromContextNeverReadsHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	context.Request.Header.Set("X-Tenant-ID", uuid.New().String())

	if id, err := tenantIDFromContext(context); err != errTenantContextMissing || id != uuid.Nil {
		t.Fatalf("id=%s err=%v, want uuid.Nil and missing context", id, err)
	}
}

// TestWriteTenantErrorStatusCodes locks the status mapping: a missing tenant
// claim is an authorization failure (403) while an unusable claim inside an
// otherwise valid token is a 401, neither is a 400.
func TestWriteTenantErrorStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "missing tenant claim is forbidden", err: errTenantContextMissing, want: http.StatusForbidden},
		{name: "invalid tenant claim is unauthorized", err: errTenantContextInvalid, want: http.StatusUnauthorized},
		{name: "unknown error falls back to forbidden", err: nil, want: http.StatusForbidden},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)

			writeTenantError(context, testCase.err)
			if recorder.Code != testCase.want {
				t.Fatalf("status=%d, want %d", recorder.Code, testCase.want)
			}
			if recorder.Code == http.StatusBadRequest {
				t.Fatal("tenant context failures must never surface as 400")
			}
		})
	}
}
