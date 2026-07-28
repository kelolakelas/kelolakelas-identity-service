package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/tutorin-id/tutorin-identity-service/internal/domain"
	"github.com/tutorin-id/tutorin-identity-service/internal/usecase"
)

type RoleHandler struct {
	roleUsecase usecase.RoleUsecase
}

func NewRoleHandler(roleUsecase usecase.RoleUsecase) *RoleHandler {
	return &RoleHandler{roleUsecase: roleUsecase}
}

func extractTenantID(c *gin.Context) (uuid.UUID, error) {
	if val, exists := c.Get("tenant_id"); exists {
		if id, ok := val.(uuid.UUID); ok && id != uuid.Nil {
			return id, nil
		}
		if idStr, ok := val.(string); ok && idStr != "" {
			return uuid.Parse(idStr)
		}
	}

	headerTenant := c.GetHeader("X-Tenant-ID")
	if headerTenant != "" {
		return uuid.Parse(headerTenant)
	}

	return uuid.Nil, errors.New("tenant ID is required")
}

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

func (h *RoleHandler) GetRoles(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
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

func (h *RoleHandler) CreateRole(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
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

	role, err := h.roleUsecase.CreateCustomRole(c.Request.Context(), tenantID, &req)
	if err != nil {
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

func (h *RoleHandler) UpdateRole(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
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

	role, err := h.roleUsecase.UpdateCustomRole(c.Request.Context(), tenantID, roleID, &req)
	if err != nil {
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

func (h *RoleHandler) DeleteRole(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
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

	err = h.roleUsecase.DeleteCustomRole(c.Request.Context(), tenantID, roleID)
	if err != nil {
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
