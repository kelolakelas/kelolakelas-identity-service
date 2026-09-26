package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type publicCatalogPolicyStub struct {
	state                 domain.PublicCatalogPolicyEvaluated
	err                   error
	closeCalls, openCalls int
	operator              uuid.UUID
}

func (s *publicCatalogPolicyStub) Evaluate(context.Context) (domain.PublicCatalogPolicyEvaluated, error) {
	return s.state, s.err
}
func (s *publicCatalogPolicyStub) Close(_ context.Context, id uuid.UUID) (domain.PublicCatalogPolicyEvaluated, error) {
	s.closeCalls++
	s.operator = id
	return s.state, s.err
}
func (s *publicCatalogPolicyStub) Open(_ context.Context, id uuid.UUID) (domain.PublicCatalogPolicyEvaluated, error) {
	s.openCalls++
	s.operator = id
	return s.state, s.err
}

func TestPublicCatalogPolicyHandlerVersionsAndErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &publicCatalogPolicyStub{state: domain.PublicCatalogPolicyEvaluated{Application: "academic", Key: "PUBLIC_CATALOG_OPEN", Environment: "platform", Open: false, AppliedVersion: 1, DesiredVersion: 2}}
	handler := NewPublicCatalogPolicyHandler(stub)
	router := gin.New()
	operator := uuid.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", operator); c.Next() })
	router.GET("/catalog-policy", handler.Get)
	router.POST("/catalog-policy/close", handler.Close)
	router.POST("/catalog-policy/open", handler.Open)
	for _, tc := range []struct{ method, path string }{{"GET", "/catalog-policy"}, {"POST", "/catalog-policy/close"}, {"POST", "/catalog-policy/open"}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"applied_version":1`) || !strings.Contains(response.Body.String(), `"desired_version":2`) {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, response.Code, response.Body.String())
		}
	}
	if stub.closeCalls != 1 || stub.openCalls != 1 || stub.operator != operator {
		t.Fatalf("mutations=%d,%d operator=%s", stub.closeCalls, stub.openCalls, stub.operator)
	}
	stub.err = domain.ErrConfigurationVersionConflict
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/catalog-policy/open", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d", response.Code)
	}
	stub.err = errors.New("database down")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/catalog-policy", nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "database down") {
		t.Fatalf("outage status=%d body=%s", response.Code, response.Body.String())
	}
}
