package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/kelolakelas/kelolakelas-identity-service/internal/migration"
)

func main() {
	directory := flag.String("dir", "seeders", "seed directory")
	file := flag.String("file", "", "optional seed file")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := migration.Seed(*directory, *file); err != nil {
		slog.Error("Seeding failed", "error", err)
		os.Exit(1)
	}
	slog.Info("Seeding completed successfully!")
}
