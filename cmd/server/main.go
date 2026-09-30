package main

import (
	"context"
	"encoding/hex"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

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
	platformFeePolicy := usecase.NewPlatformFeePolicy(repository.NewConfigurationRepository(db))
	platformFeePolicyHandler := handler.NewPlatformFeePolicyHandler(platformFeePolicy)
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
	r.Use(middleware.RequestLog(), gin.Recovery())

	// Liveness never probes dependencies; readiness does.
	r.GET("/health", healthHandler("identity-service"))
	sqlDB, err := db.DB()
	if err != nil {
		slog.Error("Failed to access database pool", "error", err)
		os.Exit(1)
	}
	r.GET("/ready", readinessHandler(sqlDB, rdb))

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
		platform.GET("/fee-policy", platformFeePolicyHandler.Get)
		platform.POST("/fee-policy", platformFeePolicyHandler.Set)
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
			protected.GET("/members/me/membership", memberHandler.GetMyMembership)
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

	// Bind both listeners before serving so a port that cannot be bound stops startup
	// with an error instead of leaving identity running with only one interface.
	grpcListener, err := net.Listen("tcp", ":50051")
	if err != nil {
		slog.Error("Failed to listen for gRPC", "error", err)
		os.Exit(1)
	}
	httpServer := newHTTPServer(cfg, r)
	httpListener, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		slog.Error("Failed to listen for HTTP", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(idgrpc.RequestLog))
	grpcHealth := health.NewServer()
	grpcHealth.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, grpcHealth)
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
	// KEL-135: academic validates a substitute tutor against the membership row
	// before assigning sessions to them, so cross-tenant, inactive, or unknown
	// member ids can never receive another tenant's sessions.
	idgrpc.RegisterMembershipServiceServer(grpcServer, idgrpc.NewMembershipServer(db))
	idgrpc.RegisterCatalogPolicyServiceServer(grpcServer, idgrpc.NewCatalogPolicyServer(publicCatalogPolicy))
	idgrpc.RegisterFeePolicyServiceServer(grpcServer, idgrpc.NewFeePolicyServer(platformFeePolicy))

	// The signal handler stays registered for the whole shutdown, so a second SIGTERM
	// does not cut the drain short; the shutdown timeout still bounds the exit.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("Starting gRPC server on port :50051")
	slog.Info("Starting identity service",
		"port", cfg.Port,
		"server_read_header_timeout_seconds", cfg.ServerReadHeaderTimeout,
		"server_read_timeout_seconds", cfg.ServerReadTimeout,
		"server_write_timeout_seconds", cfg.ServerWriteTimeout,
		"server_idle_timeout_seconds", cfg.ServerIdleTimeout,
		"server_shutdown_timeout_seconds", cfg.ServerShutdownTimeout,
	)
	if err := serveUntilDone(ctx, httpServer, httpListener, grpcServer, grpcListener, time.Duration(cfg.ServerShutdownTimeout)*time.Second); err != nil {
		slog.Error("Identity service stopped with error", "error", err)
		os.Exit(1)
	}
}
