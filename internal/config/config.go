package config

import (
	"errors"
	"os"
)

// Config holds the application configuration loaded from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	Env         string
}

// Load reads configuration from environment variables.
// Returns an error if required variables are missing.
func Load() (Config, error) {
	cfg := Config{
		Port:        getEnvOrDefault("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Env:         getEnvOrDefault("ENV", "local"),
	}

	// DATABASE_URL is required
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL environment variable is required")
	}

	return cfg, nil
}

// getEnvOrDefault retrieves an environment variable or returns a default value.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
