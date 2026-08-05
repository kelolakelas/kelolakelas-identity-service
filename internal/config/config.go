package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	DatabaseURL      string `mapstructure:"DATABASE_URL"`
	DBHost           string `mapstructure:"DB_HOST"`
	DBPort           string `mapstructure:"DB_PORT"`
	DBSSLMode        string `mapstructure:"DB_SSLMODE"`
	DBChannelBinding string `mapstructure:"DB_CHANNEL_BINDING"`
	DBUser           string `mapstructure:"DB_USER"`
	DBPassword       string `mapstructure:"DB_PASSWORD"`
	DBName           string `mapstructure:"DB_NAME"`
	RedisHost        string `mapstructure:"REDIS_HOST"`
	RedisPort        string `mapstructure:"REDIS_PORT"`
	RedisUsername    string `mapstructure:"REDIS_USERNAME"`
	RedisPassword    string `mapstructure:"REDIS_PASSWORD"`
	RedisTLS         bool   `mapstructure:"REDIS_TLS"`
	RedisDB          int    `mapstructure:"REDIS_DB"`
	JWTSecret        string `mapstructure:"JWT_SECRET"`
	Port             string `mapstructure:"PORT"`
	AppURL           string `mapstructure:"APP_URL"`
	ResendAPIKey     string `mapstructure:"RESEND_API_KEY"`
	ResendFromEmail  string `mapstructure:"RESEND_FROM_EMAIL"`
}

func LoadConfig() (Config, error) {
	// Load using godotenv just to make sure OS environment is populated,
	// though Viper's AutomaticEnv also works.
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found by godotenv")
	}

	viper.SetConfigFile(".env")
	if err := viper.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) {
			return Config{}, err
		}
		slog.Warn("No .env file found by Viper, using system environment variables")
	}

	viper.AutomaticEnv()
	for _, key := range []string{
		"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_SSLMODE", "DB_CHANNEL_BINDING", "DB_USER", "DB_PASSWORD", "DB_NAME",
		"REDIS_HOST", "REDIS_PORT", "REDIS_USERNAME", "REDIS_PASSWORD", "REDIS_TLS", "REDIS_DB", "JWT_SECRET", "PORT", "APP_URL",
		"RESEND_API_KEY", "RESEND_FROM_EMAIL",
	} {
		if err := viper.BindEnv(key); err != nil {
			return Config{}, err
		}
	}

	var parsedRedisTLS bool
	if redisTLS := viper.GetString("REDIS_TLS"); redisTLS != "" {
		var err error
		parsedRedisTLS, err = strconv.ParseBool(redisTLS)
		if err != nil {
			return Config{}, fmt.Errorf("REDIS_TLS must be a boolean: %w", err)
		}
	}
	parsedRedisDB := 0
	if redisDB := viper.GetString("REDIS_DB"); redisDB != "" {
		var err error
		parsedRedisDB, err = strconv.Atoi(redisDB)
		if err != nil || parsedRedisDB < 0 {
			return Config{}, fmt.Errorf("REDIS_DB must be a non-negative integer")
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return Config{}, err
	}
	config.RedisTLS = parsedRedisTLS
	config.RedisDB = parsedRedisDB
	if err := applyDatabaseURL(&config); err != nil {
		return Config{}, err
	}

	// Default fallback values
	if config.DBHost == "" {
		config.DBHost = "localhost"
	}
	if config.DBPort == "" {
		config.DBPort = "5432"
	}
	if config.JWTSecret == "" {
		config.JWTSecret = "supersecretjwtkey123!"
	}
	if config.DBUser == "" {
		config.DBUser = "postgres"
	}
	if config.DBPassword == "" {
		config.DBPassword = "postgres"
	}
	if config.DBName == "" {
		config.DBName = "kelolakelas_identity"
	}
	if config.Port == "" {
		config.Port = "8080"
	}
	if config.DBSSLMode == "" {
		config.DBSSLMode = "disable"
	}
	if config.DBChannelBinding == "" {
		config.DBChannelBinding = "disable"
	}
	if err := validateChannelBinding(config.DBChannelBinding); err != nil {
		return Config{}, err
	}
	if config.RedisUsername == "" {
		config.RedisUsername = "default"
	}
	if config.AppURL == "" {
		config.AppURL = "http://localhost:3000"
	}

	return config, nil
}

func applyDatabaseURL(config *Config) error {
	if config.DatabaseURL == "" {
		return nil
	}

	databaseURL, err := url.Parse(config.DatabaseURL)
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") || databaseURL.Hostname() == "" || databaseURL.Path == "" {
		return fmt.Errorf("DATABASE_URL must be a valid PostgreSQL URL")
	}
	if config.DBHost == "" {
		config.DBHost = databaseURL.Hostname()
	}
	if config.DBPort == "" {
		config.DBPort = databaseURL.Port()
		if config.DBPort == "" {
			config.DBPort = "5432"
		}
	}
	if config.DBUser == "" && databaseURL.User != nil {
		config.DBUser = databaseURL.User.Username()
	}
	if config.DBPassword == "" && databaseURL.User != nil {
		config.DBPassword, _ = databaseURL.User.Password()
	}
	if config.DBName == "" {
		config.DBName = strings.TrimPrefix(databaseURL.Path, "/")
	}
	if config.DBSSLMode == "" {
		config.DBSSLMode = databaseURL.Query().Get("sslmode")
	}
	if config.DBChannelBinding == "" {
		config.DBChannelBinding = databaseURL.Query().Get("channel_binding")
	}
	if _, err := strconv.Atoi(config.DBPort); err != nil {
		return fmt.Errorf("DATABASE_URL port must be numeric")
	}
	return nil
}

func validateChannelBinding(value string) error {
	switch value {
	case "disable", "prefer", "require":
		return nil
	default:
		return fmt.Errorf("DB_CHANNEL_BINDING must be one of disable, prefer, or require")
	}
}
