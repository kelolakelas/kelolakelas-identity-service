package main

import (
	"log/slog"
	"os"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/config"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/database"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.NewPostgresDB(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)
	if err != nil {
		slog.Error("Database connection failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Starting database seeding process...")
	if err := database.SeedAll(db); err != nil {
		slog.Error("Seeding failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Seeding completed successfully!")
}
