package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

// ListTutors godoc
// @Summary List tenant tutors
// @Description List active teacher/tutor members in the active tenant
// @Tags Tutors
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number"
// @Param page_size query int false "Items per page"
// @Param search query string false "Search tutor"
// @Param status query string false "active or inactive"
// @Success 200 {object} domain.HTTPResponse{data=domain.TutorListResponse}
// @Failure 400,401,500 {object} domain.ErrorResponse
// @Router /api/v1/tutors [get]
func (h *MemberHandler) ListTutors(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Tenant ID is required", "data": nil})
		return
	}
	query := domain.TutorQuery{Page: 1, PageSize: 20, Search: c.Query("search"), Status: c.Query("status")}
	if value := c.Query("page"); value != "" {
		query.Page, err = strconv.Atoi(value)
	}
	if err == nil {
		if value := c.Query("page_size"); value != "" {
			query.PageSize, err = strconv.Atoi(value)
		}
	}
	if err != nil || query.Page < 1 || query.PageSize < 1 || query.PageSize > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid pagination", "data": nil})
		return
	}
	result, err := h.usecase.ListTutors(c.Request.Context(), tenantID, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch tutors", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Tutors fetched successfully", "data": result})
}

type MemberHandler struct {
	usecase domain.MemberUsecase
}

func NewMemberHandler(usecase domain.MemberUsecase) *MemberHandler {
	return &MemberHandler{usecase: usecase}
}

// ListMembers godoc
// @Summary List tenant members
// @Description Fetch members belonging to the active tenant.
// @Tags Members
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number"
// @Param page_size query int false "Items per page"
// @Param search query string false "Search email or name"
// @Param status query string false "active or inactive"
// @Param role_id query string false "Role UUID"
// @Param sort query string false "joined_at, updated_at, email, or name"
// @Param order query string false "asc or desc"
// @Success 200 {object} domain.HTTPResponse{data=domain.MemberListResponse}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/members [get]
func (h *MemberHandler) ListMembers(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Tenant ID is required", "data": nil})
		return
	}

	query := domain.MemberQuery{
		Search: c.Query("search"),
		Status: c.Query("status"), Sort: c.Query("sort"), Order: c.Query("order"),
	}
	if value := c.Query("page"); value != "" {
		page, parseErr := strconv.Atoi(value)
		if parseErr != nil || page < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid page", "data": nil})
			return
		}
		query.Page = page
	}
	if value := c.Query("page_size"); value != "" {
		pageSize, parseErr := strconv.Atoi(value)
		if parseErr != nil || pageSize < 1 || pageSize > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid page_size", "data": nil})
			return
		}
		query.PageSize = pageSize
	}
	if value := c.Query("role_id"); value != "" {
		roleID, parseErr := uuid.Parse(value)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid role_id format", "data": nil})
			return
		}
		query.RoleID = &roleID
	}
	result, err := h.usecase.List(c.Request.Context(), tenantID, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch members", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Members fetched successfully", "data": result})
}

// GetMember godoc
// @Summary Get tenant member
// @Tags Members
// @Produce json
// @Security BearerAuth
// @Param id path string true "Member UUID"
// @Success 200 {object} domain.HTTPResponse{data=domain.MemberResponse}
// @Router /api/v1/members/{id} [get]
func (h *MemberHandler) GetMember(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Tenant ID is required", "data": nil})
		return
	}
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid member ID", "data": nil})
		return
	}
	member, err := h.usecase.GetByID(c.Request.Context(), tenantID, memberID)
	if errors.Is(err, domain.ErrMemberNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Member not found", "data": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch member", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Member fetched successfully", "data": member})
}

// UpdateMemberRole godoc
// @Summary Update member role
// @Tags Members
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Member UUID"
// @Param request body domain.UpdateMemberRoleRequest true "Role payload"
// @Success 200 {object} domain.HTTPResponse{data=domain.MemberResponse}
// @Failure 400,401,403,404,409 {object} domain.ErrorResponse
// @Router /api/v1/members/{id}/role [put]
func (h *MemberHandler) UpdateMemberRole(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Tenant ID is required", "data": nil})
		return
	}
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid member ID", "data": nil})
		return
	}
	var req domain.UpdateMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error(), "data": nil})
		return
	}
	callerRole, ok := c.Get("role_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Role context required", "data": nil})
		return
	}
	callerRoleID, ok := callerRole.(uuid.UUID)
	if !ok || callerRoleID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid role context", "data": nil})
		return
	}
	member, err := h.usecase.UpdateRole(c.Request.Context(), tenantID, callerRoleID, memberID, req.RoleID)
	status, message := http.StatusInternalServerError, "Failed to update member role"
	if errors.Is(err, domain.ErrMemberPermission) {
		status, message = http.StatusForbidden, "Permission denied"
	}
	if errors.Is(err, domain.ErrMemberNotFound) {
		status, message = http.StatusNotFound, "Member not found"
	}
	if errors.Is(err, domain.ErrMemberRoleConflict) {
		status, message = http.StatusConflict, "Role does not belong to tenant"
	}
	if errors.Is(err, domain.ErrMemberRoleForbidden) {
		status, message = http.StatusForbidden, "System role cannot be changed"
	}
	if err != nil {
		c.JSON(status, gin.H{"status": "error", "message": message, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Member role updated successfully", "data": member})
}

// DeleteMember godoc
// @Summary Remove a tenant member
// @Description Soft delete a member from the active tenant.
// @Tags Members
// @Produce json
// @Security BearerAuth
// @Param id path string true "Member UUID"
// @Success 200 {object} domain.HTTPResponse
// @Failure 400,401,403,404,500 {object} domain.ErrorResponse
// @Router /api/v1/members/{id} [delete]
func (h *MemberHandler) DeleteMember(c *gin.Context) {
	tenantID, err := extractTenantID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Tenant ID is required", "data": nil})
		return
	}
	memberID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid member ID", "data": nil})
		return
	}
	callerRole, ok := c.Get("role_id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Role context required", "data": nil})
		return
	}
	callerRoleID, ok := callerRole.(uuid.UUID)
	if !ok || callerRoleID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid role context", "data": nil})
		return
	}
	err = h.usecase.Delete(c.Request.Context(), tenantID, callerRoleID, memberID)
	status, message := http.StatusInternalServerError, "Failed to remove member"
	if errors.Is(err, domain.ErrMemberDeletePermission) {
		status, message = http.StatusForbidden, "Permission denied"
	}
	if errors.Is(err, domain.ErrMemberNotFound) {
		status, message = http.StatusNotFound, "Member not found"
	}
	if err != nil {
		c.JSON(status, gin.H{"status": "error", "message": message, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Member removed successfully", "data": nil})
}
