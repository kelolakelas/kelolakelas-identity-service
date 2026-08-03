package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/domain"
)

type AuthHandler struct {
	authUsecase   domain.AuthUsecase
	tenantUsecase domain.TenantUsecase
}

func NewAuthHandler(authUsecase domain.AuthUsecase, tenantUsecase domain.TenantUsecase) *AuthHandler {
	return &AuthHandler{
		authUsecase:   authUsecase,
		tenantUsecase: tenantUsecase,
	}
}

type RegisterPayload struct {
	Email     string  `json:"email" binding:"required,email"`
	Password  string  `json:"password" binding:"required,min=6"`
	FirstName string  `json:"first_name" binding:"required"`
	LastName  string  `json:"last_name" binding:"required"`
	Phone     *string `json:"phone"`
	IsParent  bool    `json:"is_parent"`
}

type LoginPayload struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Register godoc
// @Summary Register new user
// @Description Register a new end-user (student or parent)
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body RegisterPayload true "Register payload"
// @Description Registration does not auto-login. The client must call /auth/login after registration.
// @Success 201 {object} domain.HTTPResponse{data=domain.AuthUserResponse}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var payload RegisterPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	user := &domain.User{
		Email:     payload.Email,
		FirstName: payload.FirstName,
		LastName:  payload.LastName,
		Phone:     payload.Phone,
		IsParent:  payload.IsParent,
	}

	createdUser, err := h.authUsecase.Register(c.Request.Context(), user, payload.Password)
	if err != nil {
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": "User with this email already exists",
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to register user",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "User registered successfully; please log in",
		"data":    domain.AuthUserResponse{ID: createdUser.ID, Email: createdUser.Email, FirstName: createdUser.FirstName, LastName: createdUser.LastName, IsParent: createdUser.IsParent},
	})
}

// Login godoc
// @Summary User login
// @Description Authenticate user and return JWT access token
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body LoginPayload true "Login payload"
// @Success 200 {object} domain.HTTPResponse
// @Failure 400 {object} domain.ErrorResponse
// @Failure 401 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var payload LoginPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	token, user, tenantID, err := h.authUsecase.Login(c.Request.Context(), payload.Email, payload.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "Invalid email or password",
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to log in",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Logged in successfully",
		"data": gin.H{
			"token":     token,
			"user":      domain.AuthUserResponse{ID: user.ID, Email: user.Email, FirstName: user.FirstName, LastName: user.LastName, IsParent: user.IsParent, TenantID: tenantID},
			"tenant_id": tenantID,
		},
	})
}

// RegisterTenant godoc
// @Summary Register tenant and owner
// @Description Register a new tenant organization along with its initial owner user
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body domain.RegisterTenantRequest true "Register tenant payload"
// @Success 201 {object} domain.HTTPResponse{data=domain.RegisterTenantResponse}
// @Failure 400 {object} domain.ErrorResponse
// @Failure 409 {object} domain.ErrorResponse
// @Failure 500 {object} domain.ErrorResponse
// @Router /api/v1/tenants/register [post]
func (h *AuthHandler) RegisterTenant(c *gin.Context) {
	var payload domain.RegisterTenantRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": err.Error(),
			"data":    nil,
		})
		return
	}

	res, err := h.tenantUsecase.RegisterTenant(c.Request.Context(), &payload)
	if err != nil {
		if errors.Is(err, domain.ErrTenantNameAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": "Tenant with this name already exists",
				"data":    nil,
			})
			return
		}
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{
				"status":  "error",
				"message": "User with this email already exists",
				"data":    nil,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to register tenant and user: " + err.Error(),
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "Tenant and user registered successfully",
		"data":    res,
	})
}
