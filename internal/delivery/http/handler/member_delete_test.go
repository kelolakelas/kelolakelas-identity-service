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

// memberDeleteErrUsecase is a domain.MemberUsecase whose Delete returns a fixed error.
type memberDeleteErrUsecase struct {
	domain.MemberUsecase
	err error
}

func (s *memberDeleteErrUsecase) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return s.err
}

// TestDeleteMemberErrorMapping pins the status and message for every Delete outcome, including
// the KEL-81 self-removal conflict. 5xx never carries the internal error text (KEL-61).
func TestDeleteMemberErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"success", nil, http.StatusOK, "Member removed successfully"},
		{"self removal", domain.ErrMemberSelfRemoval, http.StatusConflict, "You cannot remove your own membership"},
		{"no permission", domain.ErrMemberDeletePermission, http.StatusForbidden, "Permission denied"},
		{"missing member", domain.ErrMemberNotFound, http.StatusNotFound, "Member not found"},
		{"internal", errors.New("pq: connection reset"), http.StatusInternalServerError, "Failed to remove member"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.DELETE("/members/:id", func(c *gin.Context) {
				c.Set("tenant_id", uuid.New())
				c.Set("role_id", uuid.New())
				c.Next()
			}, NewMemberHandler(&memberDeleteErrUsecase{err: test.err}).DeleteMember)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, "/members/"+uuid.NewString(), nil))

			var envelope struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
			}
			if recorder.Code != test.status || envelope.Message != test.message {
				t.Fatalf("status=%d message=%q, want %d %q", recorder.Code, envelope.Message, test.status, test.message)
			}
			wantStatus := "error"
			if test.err == nil {
				wantStatus = "success"
			}
			if envelope.Status != wantStatus {
				t.Fatalf("envelope status=%q, want %q", envelope.Status, wantStatus)
			}
		})
	}
}
