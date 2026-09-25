package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type configurationRepositoryStub struct {
	states     map[string]domain.ConfigurationState
	history    domain.ConfigurationHistory
	created    domain.ConfigurationVersion
	report     domain.ConfigurationReport
	createErr  error
	reportErr  error
	stateErr   error
	historyErr error
}

func (r *configurationRepositoryStub) ListConfigurationState(context.Context, string) (map[string]domain.ConfigurationState, error) {
	return r.states, r.stateErr
}
func (r *configurationRepositoryStub) GetConfigurationHistory(context.Context, string, string, string) (domain.ConfigurationHistory, error) {
	return r.history, r.historyErr
}
func (r *configurationRepositoryStub) CreateConfigurationVersion(_ context.Context, request domain.ConfigurationVersionRequest) (domain.ConfigurationVersion, error) {
	if r.createErr != nil {
		return domain.ConfigurationVersion{}, r.createErr
	}
	return r.created, nil
}
func (r *configurationRepositoryStub) RecordConfigurationReport(context.Context, domain.ConfigurationReportRequest) (domain.ConfigurationReport, error) {
	if r.reportErr != nil {
		return domain.ConfigurationReport{}, r.reportErr
	}
	return r.report, nil
}

func configurationHandlerRouter(repository domain.ConfigurationRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	controlPlane := usecase.NewConfigurationControlPlane(repository)
	handler := NewConfigurationHandler(controlPlane)
	operator := uuid.New()
	router := gin.New()
	addOperator := func(c *gin.Context) {
		c.Set("user_id", operator)
		c.Next()
	}
	router.GET("/configurations", addOperator, handler.Inventory)
	router.GET("/configurations/:application/:key/history", addOperator, handler.History)
	router.POST("/configurations/:application/:key/versions", addOperator, handler.CreateVersion)
	router.POST("/reports", addOperator, handler.RecordReport)
	return router
}

func TestConfigurationInventoryHandlerRedactsSensitivePlaintext(t *testing.T) {
	secret := json.RawMessage(`"must-never-leak"`)
	states := map[string]domain.ConfigurationState{
		"identity\x00JWT_SECRET": {Latest: &domain.ConfigurationVersion{
			ID: 1, Application: "identity", Environment: "prod", Key: "JWT_SECRET", Version: 1, Value: secret,
		}},
	}
	router := configurationHandlerRouter(&configurationRepositoryStub{states: states})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/configurations?environment=prod", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "must-never-leak") {
		t.Fatalf("sensitive configuration value leaked: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"not_managed"`) {
		t.Fatalf("inventory did not mark secret as externally managed: %s", response.Body.String())
	}
}

func TestConfigurationHandlerMapsValidationConflictAndPersistenceErrors(t *testing.T) {
	operator := uuid.New()
	version := domain.ConfigurationVersion{ID: 2, Application: "api-gateway", Environment: "prod", Key: "RATE_LIMIT_REQUESTS", Version: 2, Value: json.RawMessage("90"), CreatedBy: operator, CreatedAt: time.Now()}
	tests := []struct {
		name       string
		path       string
		body       string
		repository *configurationRepositoryStub
		want       int
	}{
		{name: "unknown key", path: "/configurations/api-gateway/UNKNOWN/versions", body: `{"environment":"prod","expected_version":0,"value":1}`, want: http.StatusBadRequest},
		{name: "invalid typed value", path: "/configurations/api-gateway/RATE_LIMIT_REQUESTS/versions", body: `{"environment":"prod","expected_version":0,"value":"nope"}`, want: http.StatusBadRequest},
		{name: "version conflict", path: "/configurations/api-gateway/RATE_LIMIT_REQUESTS/versions", body: `{"environment":"prod","expected_version":0,"value":90}`, repository: &configurationRepositoryStub{createErr: domain.ErrConfigurationVersionConflict}, want: http.StatusConflict},
		{name: "persistence failure", path: "/configurations/api-gateway/RATE_LIMIT_REQUESTS/versions", body: `{"environment":"prod","expected_version":0,"value":90}`, repository: &configurationRepositoryStub{createErr: errors.New("database unavailable")}, want: http.StatusInternalServerError},
		{name: "valid version", path: "/configurations/api-gateway/RATE_LIMIT_REQUESTS/versions", body: `{"environment":"prod","expected_version":1,"value":90}`, repository: &configurationRepositoryStub{created: version}, want: http.StatusCreated},
		{name: "trailing JSON rejected", path: "/configurations/api-gateway/RATE_LIMIT_REQUESTS/versions", body: `{"environment":"prod","expected_version":0,"value":90} {}`, want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := test.repository
			if repository == nil {
				repository = &configurationRepositoryStub{}
			}
			router := configurationHandlerRouter(repository)
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestConfigurationReportHandlerDoesNotClaimSuccessOnFailure(t *testing.T) {
	for _, test := range []struct {
		name       string
		repository *configurationRepositoryStub
		body       string
		want       int
	}{
		{name: "invalid status", body: `{"version_id":1,"status":"requested"}`, want: http.StatusBadRequest},
		{name: "unknown version", repository: &configurationRepositoryStub{reportErr: domain.ErrConfigurationVersionNotFound}, body: `{"version_id":999,"status":"applied"}`, want: http.StatusNotFound},
		{name: "write failure", repository: &configurationRepositoryStub{reportErr: errors.New("database unavailable")}, body: `{"version_id":1,"status":"failed"}`, want: http.StatusInternalServerError},
		{name: "record-only acknowledgement", repository: &configurationRepositoryStub{report: domain.ConfigurationReport{ID: 1, VersionID: 1, Status: domain.ConfigurationStatusApplied}}, body: `{"version_id":1,"status":"applied"}`, want: http.StatusCreated},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := test.repository
			if repository == nil {
				repository = &configurationRepositoryStub{}
			}
			router := configurationHandlerRouter(repository)
			request := httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}
