package main

import (
	"encoding/hex"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"google.golang.org/grpc"

	_ "github.com/kelolakelas/kelolakelas-identity-service/docs"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/config"
	idgrpc "github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/grpc"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/handler"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/delivery/http/middleware"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/repository"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/usecase"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/database"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/email"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/jwt"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/maps"
	pb "github.com/kelolakelas/kelolakelas-identity-service/pkg/proto/tenant"
)

// @title KelolaKelas Identity Service API
// @version 1.0
// @description Identity & Access Management Service for KelolaKelas Platform
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// Initialize JSON logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize DB
	db, err := database.NewPostgresDB(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode, cfg.DBChannelBinding)
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}

	// Initialize Redis
	var redisService *database.RedisService
	rdb, err := database.NewRedisClient(cfg.RedisHost, cfg.RedisPort, cfg.RedisUsername, cfg.RedisPassword, cfg.RedisTLS, cfg.RedisDB)
	if err != nil {
		slog.Warn("Redis connection failed (optional/non-blocking)", "error", err)
	} else if rdb != nil {
		redisService = database.NewRedisService(rdb)
		slog.Info("Redis connection established successfully")
	}

	// Initialize JWT Service (valid for 24 hours)
	jwtService := jwt.NewJWTService(cfg.JWTSecret, 24*time.Hour)

	// Initialize Services, Repositories & Usecases
	emailService := email.NewResendEmailService(cfg.ResendAPIKey, cfg.ResendFromEmail, cfg.AppURL)

	userRepo := repository.NewUserRepository(db)
	tenantRepo := repository.NewTenantRepository(db)
	invitationRepo := repository.NewInvitationRepository(db)
	rbacRepo := repository.NewRbacRepository(db)
	memberRepo := repository.NewMemberRepository(db)

	resetStore := repository.NewPasswordResetRepository(db)
	authUsecase := usecase.NewAuthUsecase(userRepo, jwtService, redisService).WithPasswordReset(resetStore, emailService, time.Duration(cfg.PasswordResetTTLMinutes)*time.Minute)
	if err := authUsecase.WithLoginProtection(repository.NewLoginAttemptStore(db), cfg.LoginFailureThreshold, time.Duration(cfg.LoginLockoutMinutes)*time.Minute); err != nil {
		slog.Error("Failed to configure login protection", "error", err)
		os.Exit(1)
	}
	factorKey, err := hex.DecodeString(cfg.PlatformFactorKey)
	if err != nil || len(factorKey) != 32 {
		slog.Error("PLATFORM_FACTOR_KEY must be 32 bytes hex")
		os.Exit(1)
	}
	platformAuth := usecase.NewPlatformAuth(userRepo, repository.NewPlatformAdminRepository(db), jwtService).WithFactor(repository.NewPlatformFactorStore(db, factorKey))
	platformHandler := handler.NewPlatformHandler(platformAuth)
	configurationHandler := handler.NewConfigurationHandler(
		usecase.NewConfigurationControlPlane(repository.NewConfigurationRepository(db)),
		usecase.NewRegistrationPolicy(repository.NewConfigurationRepository(db)),
	)
	publicCatalogPolicy := usecase.NewPublicCatalogPolicy(repository.NewConfigurationRepository(db))
	publicCatalogPolicyHandler := handler.NewPublicCatalogPolicyHandler(publicCatalogPolicy)
	mapsClient := maps.NewClient(cfg.GoogleMapsAPIKey, cfg.GoogleMapsGeocodingEnabled, time.Duration(cfg.GoogleMapsTimeoutSeconds)*time.Second)
	tenantUsecase := usecase.NewTenantUsecaseWithRegistrationPolicy(userRepo, tenantRepo, memberRepo, jwtService, redisService, mapsClient,
		usecase.NewRegistrationPolicy(repository.NewConfigurationRepository(db)))
	invitationUsecase := usecase.NewInvitationUsecase(invitationRepo, tenantRepo, userRepo, rbacRepo, memberRepo, emailService)
	roleUsecase := usecase.NewRoleUsecase(rbacRepo, memberRepo)
	memberUsecase := usecase.NewMemberUsecase(memberRepo)

	authHandler := handler.NewAuthHandler(authUsecase, tenantUsecase)
	invitationHandler := handler.NewInvitationHandler(invitationUsecase, authUsecase)
	roleHandler := handler.NewRoleHandler(roleUsecase)
	memberHandler := handler.NewMemberHandler(memberUsecase)
	creatorRequestHandler := handler.NewCreatorRequestHandler(usecase.NewCreatorRequestUsecase(repository.NewCreatorRequestRepository(db)))
	creatorDecisionHandler := handler.NewCreatorDecisionHandler(usecase.NewCreatorDecisionUsecase(repository.NewCreatorDecisionRepository(db), emailService))
	platformCreatorRequestHandler := handler.NewPlatformCreatorRequestHandler(repository.NewPlatformCreatorRequestRepository(db))
	tenantHandler := handler.NewTenantHandler(tenantUsecase)

	// Gin Router
	r := gin.New()
	r.Use(gin.Recovery())

	// Health check endpoint
	r.GET("/health", healthHandler("identity-service"))

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Route groups
	apiV1 := r.Group("/api/v1")
	{
		// Public Auth & Invitation routes
		apiV1.POST("/auth/register", authHandler.Register)
		apiV1.POST("/auth/login", authHandler.Login)
		apiV1.POST("/auth/password-reset/request", authHandler.RequestPasswordReset)
		apiV1.POST("/auth/password-reset/confirm", authHandler.ConfirmPasswordReset)
		apiV1.GET("/internal/session/check", handler.SessionCheck(jwtService, resetStore, platformAuth))
		apiV1.POST("/platform/auth/login", platformHandler.Login)
		apiV1.POST("/platform/auth/challenge", platformHandler.StartFactor)
		apiV1.POST("/platform/auth/verify", platformHandler.FinishFactor)
		apiV1.POST("/tenants/register", authHandler.RegisterTenant)
		apiV1.GET("/invitations/verify", invitationHandler.VerifyInvitation)
		apiV1.POST("/invitations/register", invitationHandler.RegisterInvitedUser)

		apiV1.GET("/platform/me", middleware.AuthMiddleware(jwtService), platformHandler.Me)

		// Platform-only routes use a live assignment check as well as the JWT claim.
		platform := apiV1.Group("/platform")
		platform.Use(middleware.AuthMiddleware(jwtService), middleware.RequireActivePlatform(platformAuth))
		platform.GET("/configurations", configurationHandler.Inventory)
		platform.GET("/configurations/:application/:key/history", configurationHandler.History)
		platform.POST("/configurations/:application/:key/versions", configurationHandler.CreateVersion)
		platform.POST("/configurations/reports", configurationHandler.RecordReport)
		platform.GET("/registration-policy", configurationHandler.RegistrationPolicy)
		platform.POST("/registration-policy/close", configurationHandler.CloseRegistration)
		platform.POST("/registration-policy/open", configurationHandler.OpenRegistration)
		platform.GET("/catalog-policy", publicCatalogPolicyHandler.Get)
		platform.POST("/catalog-policy/close", publicCatalogPolicyHandler.Close)
		platform.POST("/catalog-policy/open", publicCatalogPolicyHandler.Open)
		platform.GET("/creator-requests", platformCreatorRequestHandler.List)
		platform.POST("/creator-requests/:id/approve", creatorDecisionHandler.Approve)
		platform.POST("/creator-requests/:id/reject", creatorDecisionHandler.Reject)

		// Protected tenant routes reject tenantless platform principals.
		protected := apiV1.Group("")
		protected.Use(middleware.AuthMiddleware(jwtService), middleware.RejectTenantlessPlatform())
		{
			protected.POST("/invitations", invitationHandler.CreateInvitation)
			protected.GET("/invitations", invitationHandler.ListInvitations)
			protected.DELETE("/invitations/:id", invitationHandler.RevokeInvitation)
			protected.POST("/creator-requests", creatorRequestHandler.Create)
			protected.GET("/creator-requests", creatorRequestHandler.List)
			protected.GET("/members", memberHandler.ListMembers)
			protected.GET("/tutors", memberHandler.ListTutors)
			protected.GET("/members/:id", memberHandler.GetMember)
			protected.PUT("/members/:id/role", memberHandler.UpdateMemberRole)
			protected.DELETE("/members/:id", memberHandler.DeleteMember)
			protected.GET("/tenant/settings", tenantHandler.GetSettings)
			protected.PATCH("/tenant/settings", tenantHandler.UpdateSettings)
			protected.GET("/tenants/settings", tenantHandler.GetSettings)
			protected.PATCH("/tenants/settings", tenantHandler.UpdateSettings)
			protected.GET("/tenant/settings/location", tenantHandler.GetLocation)
			protected.PUT("/tenant/settings/location", tenantHandler.UpdateLocation)

			// Role & Permission routes
			protected.GET("/permissions", roleHandler.GetPermissions)
			protected.GET("/roles", roleHandler.GetRoles)
			protected.POST("/roles", roleHandler.CreateRole)
			protected.PUT("/roles/:id", roleHandler.UpdateRole)
			protected.DELETE("/roles/:id", roleHandler.DeleteRole)
		}
	}

	// Start gRPC Server
	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			slog.Error("Failed to listen for gRPC", "error", err)
			return
		}

		grpcServer := grpc.NewServer()
		// ADR 0002 transition window: while PERMISSION_REQUIRE_TENANT_ID is unset,
		// CheckPermission still answers academic deployments that have not been upgraded
		// yet and do not send tenant_id.
		idgrpc.RequirePermissionTenantID = cfg.PermissionRequireTenantID
		if idgrpc.RequirePermissionTenantID {
			slog.Info("CheckPermission requires tenant_id on every request")
		} else {
			slog.Warn("CheckPermission accepts requests without tenant_id (ADR 0002 transition window)")
		}
		tenantGrpcServer := idgrpc.NewTenantServiceServer(db)
		pb.RegisterTenantServiceServer(grpcServer, tenantGrpcServer)
		idgrpc.RegisterPermissionServiceServer(grpcServer, tenantGrpcServer)
		idgrpc.RegisterCatalogPolicyServiceServer(grpcServer, idgrpc.NewCatalogPolicyServer(publicCatalogPolicy))

		slog.Info("Starting gRPC server on port :50051")
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("Failed to serve gRPC", "error", err)
		}
	}()

	slog.Info("Starting identity service", "port", cfg.Port)
	if err := r.Run("0.0.0.0:" + cfg.Port); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}
