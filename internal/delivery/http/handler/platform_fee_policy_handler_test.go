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

type platformFeePolicyHandlerStub struct {
	evaluation  domain.PlatformFeePolicyEvaluated
	evaluateErr error
	setErr      error
	setCalls    int
	lastValue   domain.PlatformFeePolicyValue
}

func (s *platformFeePolicyHandlerStub) Evaluate(context.Context) (domain.PlatformFeePolicyEvaluated, error) {
	return s.evaluation, s.evaluateErr
}
func (s *platformFeePolicyHandlerStub) Set(_ context.Context, _ uuid.UUID, value domain.PlatformFeePolicyValue) (domain.PlatformFeePolicyEvaluated, error) {
	s.setCalls++
	s.lastValue = value
	if s.setErr != nil {
		return domain.PlatformFeePolicyEvaluated{}, s.setErr
	}
	if err := value.Validate(); err != nil {
		return domain.PlatformFeePolicyEvaluated{}, err
	}
	return domain.PlatformFeePolicyEvaluated{Applied: true, AppliedVersion: 1, DesiredVersion: 2, PercentBps: 0, FixedFee: 0}, nil
}

func platformFeePolicyRouter(policy domain.PlatformFeePolicy, withOperator bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	addOperator := func(c *gin.Context) {
		if withOperator {
			c.Set("user_id", uuid.New())
		}
		c.Next()
	}
	h := NewPlatformFeePolicyHandler(policy)
	router.GET("/platform/fee-policy", addOperator, h.Get)
	router.POST("/platform/fee-policy", addOperator, h.Set)
	return router
}

func postFee(router *gin.Engine, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/platform/fee-policy", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func TestPlatformFeePolicyEndpointsReadAndSet(t *testing.T) {
	stub := &platformFeePolicyHandlerStub{evaluation: domain.PlatformFeePolicyEvaluated{Applied: true, PercentBps: 500, FixedFee: 1000, AppliedVersion: 1, DesiredVersion: 2}}
	router := platformFeePolicyRouter(stub, true)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/fee-policy", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"percent_bps":500`) || !strings.Contains(body, `"fixed_fee":1000`) || !strings.Contains(body, `"applied_version":1`) || !strings.Contains(body, `"desired_version":2`) || !strings.Contains(body, `"applied":true`) {
		t.Fatalf("GET status=%d body=%s", response.Code, body)
	}

	response = postFee(router, `{"percent_bps":2000,"fixed_fee":50000}`)
	if response.Code != http.StatusOK || stub.setCalls != 1 || stub.lastValue != (domain.PlatformFeePolicyValue{PercentBps: 2000, FixedFee: 50000}) {
		t.Fatalf("set status=%d calls=%d value=%+v body=%s", response.Code, stub.setCalls, stub.lastValue, response.Body.String())
	}
	response = postFee(router, `{"percent_bps":0,"fixed_fee":0}`)
	if response.Code != http.StatusOK || stub.lastValue != (domain.PlatformFeePolicyValue{}) {
		t.Fatalf("zero set status=%d value=%+v", response.Code, stub.lastValue)
	}
}

func TestPlatformFeePolicySetRejectsInvalidInput(t *testing.T) {
	stub := &platformFeePolicyHandlerStub{}
	router := platformFeePolicyRouter(stub, true)
	for _, body := range []string{`{"percent_bps":2001,"fixed_fee":0}`, `{"percent_bps":0,"fixed_fee":50001}`, `{"percent_bps":-1,"fixed_fee":0}`} {
		if response := postFee(router, body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", body, response.Code, response.Body.String())
		}
	}
	calls := stub.setCalls
	for _, body := range []string{`{"percent_bps":5}`, `{"fixed_fee":5}`, `{}`, `{"percent_bps":1.5,"fixed_fee":0}`, `{"percent_bps":"5","fixed_fee":0}`, `not json`} {
		if response := postFee(router, body); response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", body, response.Code, response.Body.String())
		}
	}
	if stub.setCalls != calls {
		t.Fatalf("malformed bodies reached the policy: %d calls", stub.setCalls-calls)
	}
}

func TestPlatformFeePolicyErrorsMapToStatus(t *testing.T) {
	stub := &platformFeePolicyHandlerStub{setErr: domain.ErrConfigurationVersionConflict}
	router := platformFeePolicyRouter(stub, true)
	if response := postFee(router, `{"percent_bps":1,"fixed_fee":1}`); response.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d", response.Code)
	}
	stub.setErr = errors.New("database down")
	if response := postFee(router, `{"percent_bps":1,"fixed_fee":1}`); response.Code != http.StatusInternalServerError {
		t.Fatalf("outage status=%d", response.Code)
	}
	stub.evaluateErr = errors.New("database down")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/platform/fee-policy", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("GET outage status=%d", response.Code)
	}
	noOperator := platformFeePolicyRouter(&platformFeePolicyHandlerStub{}, false)
	if response := postFee(noOperator, `{"percent_bps":1,"fixed_fee":1}`); response.Code != http.StatusForbidden {
		t.Fatalf("missing operator status=%d", response.Code)
	}
}
