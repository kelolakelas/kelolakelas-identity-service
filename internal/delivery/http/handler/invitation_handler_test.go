package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

// deliveryStatusInvitationUsecase answers CreateInvitation with a prepared
// invitation so the handler's delivery message contract can be asserted for
// both outcomes of the email attempt.
type deliveryStatusInvitationUsecase struct {
	invitation *domain.TenantInvitation
	err        error
}

func (u *deliveryStatusInvitationUsecase) CreateInvitation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (*domain.TenantInvitation, error) {
	return u.invitation, u.err
}

func (*deliveryStatusInvitationUsecase) VerifyInvitation(context.Context, string) (*domain.TenantInvitation, error) {
	return nil, domain.ErrInvitationNotFound
}

func performCreateInvitation(usecase usecase.InvitationUsecase) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"role_id":"` + uuid.New().String() + `","email":"member@example.com"}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/invitations", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("tenant_id", uuid.New())
	ctx.Set("role_id", uuid.New())
	NewInvitationHandler(usecase, nil).CreateInvitation(ctx)
	return recorder
}

type createInvitationResponseBody struct {
	Status  string                   `json:"status"`
	Message string                   `json:"message"`
	Data    *domain.TenantInvitation `json:"data"`
}

func decodeCreateInvitationResponse(t *testing.T, recorder *httptest.ResponseRecorder) createInvitationResponseBody {
	t.Helper()
	var payload createInvitationResponseBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, recorder.Body.String())
	}
	return payload
}

func TestCreateInvitationCreatorGrantReturnsForbidden(t *testing.T) {
	recorder := performCreateInvitation(&deliveryStatusInvitationUsecase{err: domain.ErrCreatorGrantForbidden})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestCreateInvitationReportsSentEmail(t *testing.T) {
	invitation := &domain.TenantInvitation{Email: "member@example.com", EmailSent: true}
	recorder := performCreateInvitation(&deliveryStatusInvitationUsecase{invitation: invitation})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
	payload := decodeCreateInvitationResponse(t, recorder)
	if payload.Status != "success" {
		t.Fatalf("status = %q, want success", payload.Status)
	}
	if !strings.Contains(payload.Message, "email sent successfully") {
		t.Fatalf("message = %q, want it to state the email was sent", payload.Message)
	}
	if payload.Data == nil || !payload.Data.EmailSent {
		t.Fatalf("data.email_sent missing or false: %+v", payload.Data)
	}
}

// TestCreateInvitationReportsUndeliveredEmail is acceptance criterion 1: a
// failed delivery still stores the invitation (201) but the message must state
// the email did not go out instead of claiming success.
func TestCreateInvitationReportsUndeliveredEmail(t *testing.T) {
	invitation := &domain.TenantInvitation{Email: "member@example.com", EmailSent: false}
	recorder := performCreateInvitation(&deliveryStatusInvitationUsecase{invitation: invitation})

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (the invitation is stored either way)", recorder.Code)
	}
	payload := decodeCreateInvitationResponse(t, recorder)
	if payload.Status != "success" {
		t.Fatalf("status = %q, want success", payload.Status)
	}
	if strings.Contains(payload.Message, "email sent successfully") {
		t.Fatalf("message = %q, must not claim the email was sent", payload.Message)
	}
	if !strings.Contains(payload.Message, "could not be sent") {
		t.Fatalf("message = %q, want it to state the email was not delivered", payload.Message)
	}
	if payload.Data == nil || payload.Data.EmailSent {
		t.Fatalf("data.email_sent missing or true: %+v", payload.Data)
	}
}
