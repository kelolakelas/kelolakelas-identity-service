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

// roleUpdateErrUsecase is a domain.MemberUsecase whose UpdateRole returns a fixed error.
type roleUpdateErrUsecase struct {
	domain.MemberUsecase
	err error
}

func (s *roleUpdateErrUsecase) UpdateRole(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.MemberResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &domain.MemberResponse{}, nil
}

// TestUpdateMemberRoleErrorMapping pins the status and message for every UpdateRole outcome,
// including the KEL-79 self-change conflict.
func TestUpdateMemberRoleErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"success", nil, http.StatusOK, "Member role updated successfully"},
		{"self change", domain.ErrMemberSelfRoleChange, http.StatusConflict, "You cannot change your own member role"},
		{"role outside tenant", domain.ErrMemberRoleConflict, http.StatusConflict, "Role does not belong to tenant"},
		{"current creator", domain.ErrMemberRoleForbidden, http.StatusForbidden, "Creator role cannot be changed"},
		{"creator grant", domain.ErrCreatorGrantForbidden, http.StatusForbidden, "Creator role requires platform approval"},
		{"no permission", domain.ErrMemberPermission, http.StatusForbidden, "Permission denied"},
		{"missing member", domain.ErrMemberNotFound, http.StatusNotFound, "Member not found"},
		{"internal", errors.New("pq: connection reset"), http.StatusInternalServerError, "Failed to update member role"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.PUT("/members/:id/role", func(c *gin.Context) {
				c.Set("tenant_id", uuid.New())
				c.Set("role_id", uuid.New())
				c.Next()
			}, NewMemberHandler(&roleUpdateErrUsecase{err: test.err}).UpdateMemberRole)
			recorder := httptest.NewRecorder()
			body := bytes.NewBufferString(`{"role_id":"` + uuid.NewString() + `"}`)
			request := httptest.NewRequest(http.MethodPut, "/members/"+uuid.NewString()+"/role", body)
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)

			var envelope struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
			}
			if recorder.Code != test.status || envelope.Message != test.message {
				t.Fatalf("status=%d message=%q, want %d %q", recorder.Code, envelope.Message, test.status, test.message)
			}
		})
	}
}
