package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*testing.T)
		wantHost    string
		wantPort    string
		wantDBName  string
		wantBinding string
	}{
		{
			name: "environment variables are loaded",
			setup: func(t *testing.T) {
				t.Setenv("DB_HOST", "railway-db.internal")
				t.Setenv("PORT", "19080")
			},
			wantHost: "railway-db.internal", wantPort: "19080", wantDBName: "kelolakelas_identity", wantBinding: "disable",
		},
		{
			name:     "environment overrides defaults",
			setup:    func(t *testing.T) { t.Setenv("PORT", "49152") },
			wantHost: "localhost", wantPort: "49152", wantDBName: "kelolakelas_identity", wantBinding: "disable",
		},
		{
			name:        "missing optional variables use defaults",
			wantHost:    "localhost",
			wantPort:    "8080",
			wantDBName:  "kelolakelas_identity",
			wantBinding: "disable",
		},
		{
			name: "DATABASE_URL supplies database settings",
			setup: func(t *testing.T) {
				t.Setenv("DATABASE_URL", "postgresql://railway_user:railway_password@postgres.internal:6543/identity_db?sslmode=require&channel_binding=require")
			},
			wantHost: "postgres.internal", wantPort: "8080", wantDBName: "identity_db", wantBinding: "require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Chdir(t.TempDir())
			for _, key := range []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_SSLMODE", "DB_CHANNEL_BINDING", "DB_USER", "DB_PASSWORD", "DB_NAME", "REDIS_HOST", "REDIS_PORT", "REDIS_USERNAME", "REDIS_PASSWORD", "REDIS_TLS", "REDIS_DB", "PORT"} {
				t.Setenv(key, "")
			}
			if tt.setup != nil {
				tt.setup(t)
			}

			config, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if config.DBHost != tt.wantHost || config.Port != tt.wantPort || config.DBName != tt.wantDBName || config.DBChannelBinding != tt.wantBinding {
				t.Fatalf("config database=%s port=%s name=%s, want database=%s port=%s name=%s", config.DBHost, config.Port, config.DBName, tt.wantHost, tt.wantPort, tt.wantDBName)
			}
		})
	}
}

func TestRedisConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*testing.T)
		wantTLS      bool
		wantDB       int
		wantUsername string
		wantErr      bool
	}{
		{name: "defaults", wantUsername: "default"},
		{name: "parses TLS and database", setup: func(t *testing.T) {
			t.Setenv("REDIS_TLS", "1")
			t.Setenv("REDIS_DB", "2")
			t.Setenv("REDIS_USERNAME", "upstash")
		}, wantTLS: true, wantDB: 2, wantUsername: "upstash"},
		{name: "rejects invalid database", setup: func(t *testing.T) { t.Setenv("REDIS_DB", "not-a-number") }, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			viper.Reset()
			t.Chdir(t.TempDir())
			for _, key := range []string{"DATABASE_URL", "DB_CHANNEL_BINDING", "JWT_SECRET", "REDIS_TLS", "REDIS_DB", "REDIS_USERNAME"} {
				t.Setenv(key, "")
			}
			if test.setup != nil {
				test.setup(t)
			}
			config, err := LoadConfig()
			if test.wantErr {
				if err == nil {
					t.Fatal("expected configuration error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.RedisTLS != test.wantTLS || config.RedisDB != test.wantDB || config.RedisUsername != test.wantUsername {
				t.Fatalf("redis config=%+v", config)
			}
		})
	}
}

func TestChannelBindingEnvironmentOverridesDatabaseURL(t *testing.T) {
	viper.Reset()
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost/db?channel_binding=require")
	t.Setenv("DB_CHANNEL_BINDING", "disable")
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.DBChannelBinding != "disable" {
		t.Fatalf("channel binding=%q, want disable", config.DBChannelBinding)
	}
}

func TestLoadConfigRejectsInvalidChannelBinding(t *testing.T) {
	viper.Reset()
	t.Chdir(t.TempDir())
	t.Setenv("DB_CHANNEL_BINDING", "invalid")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected invalid channel binding configuration error")
	}
}

func TestLoadConfigReadsDisabledChannelBindingFromDatabaseURL(t *testing.T) {
	viper.Reset()
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost/db?sslmode=disable&channel_binding=disable")
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.DBChannelBinding != "disable" {
		t.Fatalf("channel binding=%q, want disable", config.DBChannelBinding)
	}
}
