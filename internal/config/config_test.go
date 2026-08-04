package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*testing.T)
		wantHost   string
		wantPort   string
		wantDBName string
	}{
		{
			name: "environment variables are loaded",
			setup: func(t *testing.T) {
				t.Setenv("DB_HOST", "railway-db.internal")
				t.Setenv("PORT", "19080")
			},
			wantHost: "railway-db.internal", wantPort: "19080", wantDBName: "kelolakelas_identity",
		},
		{
			name:     "environment overrides defaults",
			setup:    func(t *testing.T) { t.Setenv("PORT", "49152") },
			wantHost: "localhost", wantPort: "49152", wantDBName: "kelolakelas_identity",
		},
		{
			name:       "missing optional variables use defaults",
			wantHost:   "localhost",
			wantPort:   "8080",
			wantDBName: "kelolakelas_identity",
		},
		{
			name: "DATABASE_URL supplies database settings",
			setup: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgresql://railway_user:railway_password@postgres.internal:6543/identity_db?sslmode=require")
			},
			wantHost: "postgres.internal", wantPort: "8080", wantDBName: "identity_db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Chdir(t.TempDir())
			for _, key := range []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_SSLMODE", "DB_USER", "DB_PASSWORD", "DB_NAME", "PORT"} {
				t.Setenv(key, "")
			}
			if tt.setup != nil {
				tt.setup(t)
			}

			config, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if config.DBHost != tt.wantHost || config.Port != tt.wantPort || config.DBName != tt.wantDBName {
				t.Fatalf("config database=%s port=%s name=%s, want database=%s port=%s name=%s", config.DBHost, config.Port, config.DBName, tt.wantHost, tt.wantPort, tt.wantDBName)
			}
		})
	}
}
