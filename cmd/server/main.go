package main

import (
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

	authUsecase := usecase.NewAuthUsecase(userRepo, jwtService, redisService)
	platformAuth := usecase.NewPlatformAuth(userRepo, repository.NewPlatformAdminRepository(db), jwtService)
	platformHandler := handler.NewPlatformHandler(platformAuth)
	configurationHandler := handler.NewConfigurationHandler(usecase.NewConfigurationControlPlane(repository.NewConfigurationRepository(db)))
	mapsClient := maps.NewClient(cfg.GoogleMapsAPIKey, cfg.GoogleMapsGeocodingEnabled, time.Duration(cfg.GoogleMapsTimeoutSeconds)*time.Second)
	tenantUsecase := usecase.NewTenantUsecase(userRepo, tenantRepo, memberRepo, jwtService, redisService, mapsClient)
	invitationUsecase := usecase.NewInvitationUsecase(invitationRepo, tenantRepo, userRepo, rbacRepo, memberRepo, emailService)
	roleUsecase := usecase.NewRoleUsecase(rbacRepo, memberRepo)
	memberUsecase := usecase.NewMemberUsecase(memberRepo)

	authHandler := handler.NewAuthHandler(authUsecase, tenantUsecase)
	invitationHandler := handler.NewInvitationHandler(invitationUsecase, authUsecase)
	roleHandler := handler.NewRoleHandler(roleUsecase)
	memberHandler := handler.NewMemberHandler(memberUsecase)
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
		apiV1.POST("/platform/auth/login", platformHandler.Login)
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

		// Protected tenant routes reject tenantless platform principals.
		protected := apiV1.Group("")
		protected.Use(middleware.AuthMiddleware(jwtService), middleware.RejectTenantlessPlatform())
		{
			protected.POST("/invitations", invitationHandler.CreateInvitation)
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
