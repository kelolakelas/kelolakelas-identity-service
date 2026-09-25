package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type permissionDeniedInvitationUsecase struct {
	callerRoleID uuid.UUID
}

func (u *permissionDeniedInvitationUsecase) CreateInvitation(_ context.Context, _ uuid.UUID, callerRoleID, _ uuid.UUID, _ string) (*domain.TenantInvitation, error) {
	u.callerRoleID = callerRoleID
	return nil, domain.ErrPermissionDenied
}

func (*permissionDeniedInvitationUsecase) VerifyInvitation(context.Context, string) (*domain.TenantInvitation, error) {
	return nil, nil
}
func (*permissionDeniedInvitationUsecase) ListInvitations(context.Context, uuid.UUID, uuid.UUID) ([]domain.TenantInvitation, error) {
	return nil, domain.ErrPermissionDenied
}
func (*permissionDeniedInvitationUsecase) RevokeInvitation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return domain.ErrPermissionDenied
}

type permissionDeniedRoleUsecase struct {
	callerRoleIDs []uuid.UUID
}

func (*permissionDeniedRoleUsecase) FetchAllPermissions(context.Context) ([]domain.PermissionResponse, error) {
	return nil, nil
}

func (*permissionDeniedRoleUsecase) FetchTenantRoles(context.Context, uuid.UUID) ([]domain.RoleResponse, error) {
	return nil, nil
}

func (u *permissionDeniedRoleUsecase) CreateCustomRole(_ context.Context, _ uuid.UUID, callerRoleID uuid.UUID, _ *domain.CreateRoleRequest) (*domain.RoleResponse, error) {
	u.callerRoleIDs = append(u.callerRoleIDs, callerRoleID)
	return nil, domain.ErrPermissionDenied
}

func (u *permissionDeniedRoleUsecase) UpdateCustomRole(_ context.Context, _ uuid.UUID, callerRoleID, _ uuid.UUID, _ *domain.UpdateRoleRequest) (*domain.RoleResponse, error) {
	u.callerRoleIDs = append(u.callerRoleIDs, callerRoleID)
	return nil, domain.ErrPermissionDenied
}

func (u *permissionDeniedRoleUsecase) DeleteCustomRole(_ context.Context, _ uuid.UUID, callerRoleID, _ uuid.UUID) error {
	u.callerRoleIDs = append(u.callerRoleIDs, callerRoleID)
	return domain.ErrPermissionDenied
}

type permissionDeniedTenantUsecase struct {
	callerRoleIDs []uuid.UUID
}

func (*permissionDeniedTenantUsecase) RegisterTenant(context.Context, *domain.RegisterTenantRequest) (*domain.RegisterTenantResponse, error) {
	return nil, nil
}

func (*permissionDeniedTenantUsecase) GetTenantByID(context.Context, uuid.UUID) (*domain.Tenant, error) {
	return nil, nil
}

func (u *permissionDeniedTenantUsecase) UpdateTenantSettings(_ context.Context, _, callerRoleID uuid.UUID, _ *domain.UpdateTenantSettingsRequest) (*domain.Tenant, error) {
	u.callerRoleIDs = append(u.callerRoleIDs, callerRoleID)
	return nil, domain.ErrPermissionDenied
}

func (*permissionDeniedTenantUsecase) GetTenantLocation(context.Context, uuid.UUID) (*domain.TenantLocation, error) {
	return nil, nil
}

func (u *permissionDeniedTenantUsecase) UpdateTenantLocation(_ context.Context, _, callerRoleID uuid.UUID, _ *domain.UpdateTenantLocationRequest) (*domain.TenantLocation, error) {
	u.callerRoleIDs = append(u.callerRoleIDs, callerRoleID)
	return nil, domain.ErrPermissionDenied
}

func permissionTestContext(method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("tenant_id", uuid.New())
	return context, recorder
}

func TestAdministrativeMutationsWithoutRoleReturnForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	targetRoleID := uuid.New()

	t.Run("invitation", func(t *testing.T) {
		usecase := &permissionDeniedInvitationUsecase{}
		context, recorder := permissionTestContext(http.MethodPost, `{"role_id":"`+targetRoleID.String()+`","email":"member@example.com"}`)
		NewInvitationHandler(usecase, nil).CreateInvitation(context)
		if recorder.Code != http.StatusForbidden || usecase.callerRoleID != uuid.Nil {
			t.Fatalf("status=%d caller_role_id=%s, want 403 and nil role", recorder.Code, usecase.callerRoleID)
		}
	})

	t.Run("role CRUD", func(t *testing.T) {
		usecase := &permissionDeniedRoleUsecase{}
		handler := NewRoleHandler(usecase)
		context, recorder := permissionTestContext(http.MethodPost, `{"name":"Staff","permission_ids":["`+uuid.New().String()+`"]}`)
		handler.CreateRole(context)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("create status=%d, want 403", recorder.Code)
		}

		context, recorder = permissionTestContext(http.MethodPut, `{"name":"Staff","permission_ids":["`+uuid.New().String()+`"]}`)
		context.Params = gin.Params{{Key: "id", Value: targetRoleID.String()}}
		handler.UpdateRole(context)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("update status=%d, want 403", recorder.Code)
		}

		context, recorder = permissionTestContext(http.MethodDelete, "")
		context.Params = gin.Params{{Key: "id", Value: targetRoleID.String()}}
		handler.DeleteRole(context)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("delete status=%d, want 403", recorder.Code)
		}
		for _, callerRoleID := range usecase.callerRoleIDs {
			if callerRoleID != uuid.Nil {
				t.Fatalf("caller_role_id=%s, want nil role", callerRoleID)
			}
		}
	})

	t.Run("tenant settings and location", func(t *testing.T) {
		usecase := &permissionDeniedTenantUsecase{}
		handler := NewTenantHandler(usecase)
		context, recorder := permissionTestContext(http.MethodPatch, `{"name":"Tenant"}`)
		handler.UpdateSettings(context)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("settings status=%d, want 403", recorder.Code)
		}

		context, recorder = permissionTestContext(http.MethodPut, `{"address":"Jakarta"}`)
		handler.UpdateLocation(context)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("location status=%d, want 403", recorder.Code)
		}
		for _, callerRoleID := range usecase.callerRoleIDs {
			if callerRoleID != uuid.Nil {
				t.Fatalf("caller_role_id=%s, want nil role", callerRoleID)
			}
		}
	})
}

// invalidInvitableRoleInvitationUsecase reports the role-scope validation error so the handler
// mapping can be asserted without a database.
type invalidInvitableRoleInvitationUsecase struct{}

func (*invalidInvitableRoleInvitationUsecase) CreateInvitation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) (*domain.TenantInvitation, error) {
	return nil, domain.ErrInvitationRoleInvalid
}

func (*invalidInvitableRoleInvitationUsecase) VerifyInvitation(context.Context, string) (*domain.TenantInvitation, error) {
	return nil, nil
}
func (*invalidInvitableRoleInvitationUsecase) ListInvitations(context.Context, uuid.UUID, uuid.UUID) ([]domain.TenantInvitation, error) {
	return nil, nil
}
func (*invalidInvitableRoleInvitationUsecase) RevokeInvitation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}

// TestCreateInvitationWithForeignRoleReturnsBadRequest covers the delivery-level part of
// KEL-20: inviting someone with a role owned by another tenant is a client error, not a 500.
func TestCreateInvitationWithForeignRoleReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	context, recorder := permissionTestContext(http.MethodPost, `{"role_id":"`+uuid.New().String()+`","email":"member@example.com"}`)
	NewInvitationHandler(&invalidInvitableRoleInvitationUsecase{}, nil).CreateInvitation(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", recorder.Code)
	}
}
