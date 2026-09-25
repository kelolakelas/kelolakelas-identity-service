package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type ConfigurationHandler struct {
	controlPlane *usecase.ConfigurationControlPlane
	// registrationPolicy is required; every constructor sets it. It may be
	// nil only in tests that never exercise the registration-policy actions.
	registrationPolicy domain.RegistrationPolicy
}

func NewConfigurationHandler(controlPlane *usecase.ConfigurationControlPlane, registrationPolicy domain.RegistrationPolicy) *ConfigurationHandler {
	return &ConfigurationHandler{controlPlane: controlPlane, registrationPolicy: registrationPolicy}
}

// Inventory godoc
// @Summary Read configuration inventory
// @Description Lists metadata and current version state for all five applications. Sensitive values are never managed or returned.
// @Tags Platform Configuration
// @Security BearerAuth
// @Param environment query string true "Deployment environment"
// @Success 200 {object} domain.HTTPResponse{data=usecase.ConfigurationInventory}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Failure 503 {object} domain.ErrorResponse
// @Router /api/v1/platform/configurations [get]
func decodeConfigurationJSON(c *gin.Context, destination any) error {
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("request body must contain exactly one JSON value")
	}
	return nil
}

func (h *ConfigurationHandler) Inventory(c *gin.Context) {
	inventory, err := h.controlPlane.Inventory(c.Request.Context(), c.Query("environment"))
	if err != nil {
		if usecase.IsConfigurationValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error(), "data": nil})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Configuration inventory unavailable", "data": nil})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": inventory})
}

// ListConfigurationVersions godoc
// @Summary Read configuration version and report history
// @Tags Platform Configuration
// @Security BearerAuth
// @Param application path string true "Application identifier"
// @Param key path string true "Configuration key"
// @Param environment query string true "Deployment environment"
// @Success 200 {object} domain.HTTPResponse{data=domain.ConfigurationHistory}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/configurations/{application}/{key}/history [get]
func (h *ConfigurationHandler) History(c *gin.Context) {
	history, err := h.controlPlane.History(c.Request.Context(), c.Param("application"), c.Query("environment"), c.Param("key"))
	if err != nil {
		if usecase.IsConfigurationValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error(), "data": nil})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Configuration history unavailable", "data": nil})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": history})
}

type createConfigurationVersionPayload struct {
	Environment     string          `json:"environment" binding:"required"`
	ExpectedVersion *int64          `json:"expected_version" binding:"required,min=0"`
	Value           json.RawMessage `json:"value"`
	RollbackVersion *int64          `json:"rollback_version_id,omitempty" binding:"omitempty,min=1"`
}

// CreateVersion godoc
// @Summary Create a requested configuration version
// @Description Stores a validated non-secret requested value or creates a new version from a manual rollback target. This endpoint never applies runtime configuration.
// @Tags Platform Configuration
// @Security BearerAuth
// @Param application path string true "Application identifier"
// @Param key path string true "Configuration key"
// @Param request body createConfigurationVersionPayload true "Configuration version request"
// @Success 201 {object} domain.HTTPResponse{data=domain.ConfigurationVersion}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/configurations/{application}/{key}/versions [post]
func (h *ConfigurationHandler) CreateVersion(c *gin.Context) {
	var payload createConfigurationVersionPayload
	if err := decodeConfigurationJSON(c, &payload); err != nil || payload.ExpectedVersion == nil || len(payload.Value) == 0 && payload.RollbackVersion == nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid configuration version payload", "data": nil})
		return
	}
	operator, ok := c.Get("user_id")
	operatorID, valid := operator.(uuid.UUID)
	if !ok || !valid || operatorID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
		return
	}
	version, err := h.controlPlane.CreateVersion(c.Request.Context(), domain.ConfigurationVersionRequest{
		Application:         c.Param("application"),
		Environment:         payload.Environment,
		Key:                 c.Param("key"),
		ExpectedVersion:     *payload.ExpectedVersion,
		Value:               payload.Value,
		RollbackOfVersionID: payload.RollbackVersion,
		CreatedBy:           operatorID,
	})
	if errors.Is(err, domain.ErrConfigurationVersionConflict) {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "Configuration version conflict; refresh the inventory and retry", "data": nil})
		return
	}
	if err != nil {
		if errors.Is(err, domain.ErrConfigurationVersionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Configuration rollback target not found", "data": nil})
		} else if usecase.IsConfigurationValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error(), "data": nil})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Configuration version was not created", "data": nil})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": version})
}

type configurationReportPayload struct {
	VersionID int64                      `json:"version_id" binding:"required,min=1"`
	Status    domain.ConfigurationStatus `json:"status" binding:"required"`
}

// RecordReport godoc
// @Summary Record a configuration application status acknowledgement
// @Description Records a platform operator's applied, failed, or rollback acknowledgement. This endpoint does not apply or roll back runtime configuration.
// @Tags Platform Configuration
// @Security BearerAuth
// @Param request body configurationReportPayload true "Configuration report"
// @Success 201 {object} domain.HTTPResponse{data=domain.ConfigurationReport}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/configurations/reports [post]
func (h *ConfigurationHandler) RecordReport(c *gin.Context) {
	var payload configurationReportPayload
	if err := decodeConfigurationJSON(c, &payload); err != nil || payload.VersionID < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid configuration report payload", "data": nil})
		return
	}
	operator, ok := c.Get("user_id")
	operatorID, valid := operator.(uuid.UUID)
	if !ok || !valid || operatorID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
		return
	}
	report, err := h.controlPlane.RecordReport(c.Request.Context(), domain.ConfigurationReportRequest{
		VersionID:  payload.VersionID,
		Status:     payload.Status,
		ReportedBy: operatorID,
	})
	if errors.Is(err, domain.ErrConfigurationVersionNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Configuration version not found", "data": nil})
		return
	}
	if err != nil {
		if usecase.IsConfigurationValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error(), "data": nil})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Configuration report was not recorded", "data": nil})
		}
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "data": report})
}

// RegistrationPolicy godoc
// @Summary Read the new-tenant registration policy
// @Description Returns the effective open/closed state with the applied and desired configuration versions that decided it.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.RegistrationPolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/registration-policy [get]
func (h *ConfigurationHandler) RegistrationPolicy(c *gin.Context) {
	evaluated, err := h.registrationPolicy.Evaluate(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Registration policy unavailable", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": evaluated})
}

// CloseRegistration godoc
// @Summary Close new-tenant registration
// @Description Appends a desired configuration version that closes registration. Registration stops once the operator acknowledges the version as applied.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.RegistrationPolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/registration-policy/close [post]
func (h *ConfigurationHandler) CloseRegistration(c *gin.Context) {
	h.mutateRegistration(c, false)
}

// OpenRegistration godoc
// @Summary Reopen new-tenant registration
// @Description Appends a desired configuration version that reopens registration. Registration resumes once the operator acknowledges the version as applied.
// @Tags Platform Configuration
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=domain.RegistrationPolicyEvaluated}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/platform/registration-policy/open [post]
func (h *ConfigurationHandler) OpenRegistration(c *gin.Context) {
	h.mutateRegistration(c, true)
}

func (h *ConfigurationHandler) mutateRegistration(c *gin.Context, open bool) {
	operator, ok := c.Get("user_id")
	operatorID, valid := operator.(uuid.UUID)
	if !ok || !valid || operatorID == uuid.Nil {
		c.JSON(http.StatusForbidden, gin.H{"status": "error", "message": "Platform access required", "data": nil})
		return
	}
	var evaluated domain.RegistrationPolicyEvaluated
	var err error
	if open {
		evaluated, err = h.registrationPolicy.Open(c.Request.Context(), operatorID)
	} else {
		evaluated, err = h.registrationPolicy.Close(c.Request.Context(), operatorID)
	}
	if errors.Is(err, domain.ErrConfigurationVersionConflict) {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "Registration policy changed concurrently; read it again and retry", "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Registration policy was not updated", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": evaluated})
}
