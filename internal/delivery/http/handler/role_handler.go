package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
)

type RoleHandler struct {
	roleUsecase usecase.RoleUsecase
}

func NewRoleHandler(roleUsecase usecase.RoleUsecase) *RoleHandler {
	return &RoleHandler{roleUsecase: roleUsecase}
}

func extractCallerRoleID(c *gin.Context) uuid.UUID {
	roleID, ok := c.Get("role_id")
	if !ok {
		return uuid.Nil
	}
	parsedRoleID, ok := roleID.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return parsedRoleID
}

// GetPermissions godoc
// @Summary Get all system permissions
// @Description Retrieve a list of all available permissions in the system
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=[]domain.PermissionResponse}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/permissions [get]
func (h *RoleHandler) GetPermissions(c *gin.Context) {
	permissions, err := h.roleUsecase.FetchAllPermissions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to fetch permissions",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Permissions retrieved successfully",
		"data":    permissions,
	})
}

// GetRoles godoc
// @Summary Get tenant roles
// @Description Fetch all custom and system roles for the tenant carried by the caller's access token
// @Description The tenant is resolved from the verified JWT claim only; any X-Tenant-ID header is ignored
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} domain.HTTPResponse{data=[]domain.RoleResponse}
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/roles [get]
func (h *RoleHandler) GetRoles(c *gin.Context) {
	tenantID, err := tenantIDFromContext(c)
	if err != nil {
		writeTenantError(c, err)
		return
	}

	roles, err := h.roleUsecase.FetchTenantRoles(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to fetch roles",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Roles retrieved successfully",
		"data":    roles,
	})
}

// CreateRole godoc
// @Summary Create a custom role
// @Description Create a new custom role with assigned permissions for the tenant carried by the caller's access token
// @Description The tenant is resolved from the verified JWT claim only; any X-Tenant-ID header is ignored
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body domain.CreateRoleRequest true "Create role payload"
// @Success 201 {object} domain.HTTPResponse{data=domain.RoleResponse}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/roles [post]
func (h *RoleHandler) CreateRole(c *gin.Context) {
	tenantID, err := tenantIDFromContext(c)
	if err != nil {
		writeTenantError(c, err)
		return
	}

	var req domain.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	role, err := h.roleUsecase.CreateCustomRole(c.Request.Context(), tenantID, extractCallerRoleID(c), &req)
	if err != nil {
		if errors.Is(err, domain.ErrPermissionDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Permission denied",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrRoleNameExists) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to create role",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "Role created successfully",
		"data":    role,
	})
}

// UpdateRole godoc
// @Summary Update a custom role
// @Description Update name, description, or permissions of an existing custom role
// @Description The tenant is resolved from the verified JWT claim only; any X-Tenant-ID header is ignored
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Role ID (UUID)"
// @Param request body domain.UpdateRoleRequest true "Update role payload"
// @Success 200 {object} domain.HTTPResponse{data=domain.RoleResponse}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/roles/{id} [put]
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	tenantID, err := tenantIDFromContext(c)
	if err != nil {
		writeTenantError(c, err)
		return
	}

	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid role ID format",
			"data":    nil,
		})
		return
	}

	var req domain.UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	role, err := h.roleUsecase.UpdateCustomRole(c.Request.Context(), tenantID, extractCallerRoleID(c), roleID, &req)
	if err != nil {
		if errors.Is(err, domain.ErrPermissionDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Permission denied",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrRoleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrSystemRoleCannotBeModified) || errors.Is(err, domain.ErrForbiddenRoleAccess) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrRoleNameExists) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to update role",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Role updated successfully",
		"data":    role,
	})
}

// DeleteRole godoc
// @Summary Delete a custom role
// @Description Delete an existing custom role from a tenant
// @Description The tenant is resolved from the verified JWT claim only; any X-Tenant-ID header is ignored
// @Tags Roles & Permissions
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Role ID (UUID)"
// @Success 200 {object} domain.HTTPResponse
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 403 {object} domain.ErrorResponse
// @Failure 404 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/roles/{id} [delete]
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	tenantID, err := tenantIDFromContext(c)
	if err != nil {
		writeTenantError(c, err)
		return
	}

	roleIDStr := c.Param("id")
	roleID, err := uuid.Parse(roleIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Invalid role ID format",
			"data":    nil,
		})
		return
	}

	err = h.roleUsecase.DeleteCustomRole(c.Request.Context(), tenantID, extractCallerRoleID(c), roleID)
	if err != nil {
		if errors.Is(err, domain.ErrPermissionDenied) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": "Permission denied",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrRoleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrSystemRoleCannotBeModified) || errors.Is(err, domain.ErrForbiddenRoleAccess) {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrRoleAssignedToActiveMembers) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": err.Error(),
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to delete role",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Role deleted successfully",
		"data":    nil,
	})
}
