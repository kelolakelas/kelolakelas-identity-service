package handler

import (
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

// memberListSortStubUsecase records the MemberQuery that reaches the usecase so
// the KEL-165 sort normalization can be pinned at the HTTP boundary.
type memberListSortStubUsecase struct {
	domain.MemberUsecase
	query  domain.MemberQuery
	calls  int
	result *domain.MemberListResponse
	err    error
}

func (s *memberListSortStubUsecase) List(_ context.Context, _ uuid.UUID, query domain.MemberQuery) (*domain.MemberListResponse, error) {
	s.calls++
	s.query = query
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &domain.MemberListResponse{Items: []domain.MemberResponse{}, Pagination: domain.Pagination{}}, nil
}

func serveMemberList(stub *memberListSortStubUsecase, target string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/members", func(c *gin.Context) {
		c.Set("tenant_id", uuid.New())
		c.Next()
	}, NewMemberHandler(stub).ListMembers)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func decodeMemberListEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) (string, string) {
	t.Helper()
	var envelope struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
	}
	return envelope.Status, envelope.Message
}

// TestListMembersSortValidation pins the KEL-165 delivery contract: an empty
// sort reaches the usecase as joined_at (default asc direction untouched), every
// supported sort passes through unchanged, and an unknown sort is a 400
// validation error that never reaches the usecase.
func TestListMembersSortValidation(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantSort  string
		wantOrder string
		wantCalls int
		wantCode  int
	}{
		{name: "empty sort defaults to joined_at", target: "/api/v1/members", wantSort: "joined_at", wantCalls: 1, wantCode: http.StatusOK},
		{name: "whitespace sort is trimmed", target: "/api/v1/members?sort=%20joined_at%20", wantSort: "joined_at", wantCalls: 1, wantCode: http.StatusOK},
		{name: "joined_at passes through", target: "/api/v1/members?sort=joined_at", wantSort: "joined_at", wantCalls: 1, wantCode: http.StatusOK},
		{name: "created_at alias passes through", target: "/api/v1/members?sort=created_at", wantSort: "created_at", wantCalls: 1, wantCode: http.StatusOK},
		{name: "updated_at passes through", target: "/api/v1/members?sort=updated_at", wantSort: "updated_at", wantCalls: 1, wantCode: http.StatusOK},
		{name: "email passes through", target: "/api/v1/members?sort=email", wantSort: "email", wantCalls: 1, wantCode: http.StatusOK},
		{name: "name passes through", target: "/api/v1/members?sort=name", wantSort: "name", wantCalls: 1, wantCode: http.StatusOK},
		{name: "desc order without sort keeps joined_at", target: "/api/v1/members?order=desc", wantSort: "joined_at", wantOrder: "desc", wantCalls: 1, wantCode: http.StatusOK},
		{name: "unknown sort is rejected", target: "/api/v1/members?sort=foobar", wantCalls: 0, wantCode: http.StatusBadRequest},
		{name: "unknown sort with spaces is rejected", target: "/api/v1/members?sort=%20foobar%20", wantCalls: 0, wantCode: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := &memberListSortStubUsecase{}
			recorder := serveMemberList(stub, test.target)
			if recorder.Code != test.wantCode {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), test.wantCode)
			}
			if stub.calls != test.wantCalls {
				t.Fatalf("usecase calls=%d, want %d", stub.calls, test.wantCalls)
			}
			status, message := decodeMemberListEnvelope(t, recorder)
			if test.wantCode == http.StatusBadRequest {
				if status != "error" || message != "Invalid sort" {
					t.Fatalf("status=%q message=%q, want error/Invalid sort", status, message)
				}
				return
			}
			if status != "success" {
				t.Fatalf("envelope status=%q body=%s, want success", status, recorder.Body.String())
			}
			if stub.query.Sort != test.wantSort {
				t.Fatalf("sort=%q, want %q", stub.query.Sort, test.wantSort)
			}
			if stub.query.Order != test.wantOrder {
				t.Fatalf("order=%q, want %q", stub.query.Order, test.wantOrder)
			}
		})
	}
}

// TestListMembersRepositoryErrorIsServerError keeps a usecase failure visible
// as a 500 that never leaks the internal error text.
func TestListMembersRepositoryErrorIsServerError(t *testing.T) {
	stub := &memberListSortStubUsecase{err: errors.New("pq: connection reset")}
	recorder := serveMemberList(stub, "/api/v1/members")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s, want 500", recorder.Code, recorder.Body.String())
	}
	status, message := decodeMemberListEnvelope(t, recorder)
	if status != "error" || message != "Failed to fetch members" {
		t.Fatalf("status=%q message=%q, want error/Failed to fetch members", status, message)
	}
}
