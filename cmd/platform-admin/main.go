// Command platform-admin grants or restores a platform assignment for an
// existing user. It does not create users or accept passwords.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/kelolakelas/kelolakelas-identity-service/internal/config"
	"github.com/kelolakelas/kelolakelas-identity-service/pkg/database"
	"gorm.io/gorm"
)

func grant(ctx context.Context, db *gorm.DB, userID uuid.UUID) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("users").Where("id = ? AND deleted_at IS NULL", userID).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Exec(`INSERT INTO platform_admin_assignments (user_id, is_active)
            VALUES (?, true) ON CONFLICT (user_id) DO UPDATE
            SET is_active = true, updated_at = now()`, userID).Error
	})
}

func main() {
	id := flag.String("user-id", "", "existing user UUID to grant/recover")
	flag.Parse()
	userID, err := uuid.Parse(*id)
	if err != nil || userID == uuid.Nil {
		slog.Error("valid -user-id required")
		os.Exit(1)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	db, err := database.NewPostgresDB(cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBSSLMode, cfg.DBChannelBinding)
	if err != nil {
		slog.Error("database failed", "error", err)
		os.Exit(1)
	}
	if err := grant(context.Background(), db, userID); err != nil {
		slog.Error("grant failed", "error", err)
		os.Exit(1)
	}
	slog.Info("platform assignment active", "user_id", userID)
}
