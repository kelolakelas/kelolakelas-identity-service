package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

type Config struct {
	DBHost          string `mapstructure:"DB_HOST"`
	DBPort          string `mapstructure:"DB_PORT"`
	DBUser          string `mapstructure:"DB_USER"`
	DBPassword      string `mapstructure:"DB_PASSWORD"`
	DBName          string `mapstructure:"DB_NAME"`
	RedisHost       string `mapstructure:"REDIS_HOST"`
	RedisPort       string `mapstructure:"REDIS_PORT"`
	RedisPassword   string `mapstructure:"REDIS_PASSWORD"`
	JWTSecret       string `mapstructure:"JWT_SECRET"`
	Port            string `mapstructure:"PORT"`
	ResendAPIKey    string `mapstructure:"RESEND_API_KEY"`
	ResendFromEmail string `mapstructure:"RESEND_FROM_EMAIL"`
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

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return Config{}, err
	}

	// Default fallback values
	if config.JWTSecret == "" {
		config.JWTSecret = "supersecretjwtkey123!"
	}
	if config.Port == "" {
		config.Port = "8080"
	}

	return config, nil
}
