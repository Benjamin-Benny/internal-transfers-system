package config

import (
	"os"
	"testing"
)

func TestLoad_WithAllEnvVars(t *testing.T) {
	// Set up environment variables
	os.Setenv("PORT", "9000")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/testdb")
	os.Setenv("ENV", "production")
	defer cleanupEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.Port != "9000" {
		t.Errorf("expected Port=9000, got %s", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://test:test@localhost:5432/testdb" {
		t.Errorf("expected DatabaseURL=postgres://test:test@localhost:5432/testdb, got %s", cfg.DatabaseURL)
	}
	if cfg.Env != "production" {
		t.Errorf("expected Env=production, got %s", cfg.Env)
	}
}

func TestLoad_WithDefaults(t *testing.T) {
	// Set only required DATABASE_URL
	os.Setenv("DATABASE_URL", "postgres://app:app@localhost:5432/transfers")
	defer cleanupEnv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("expected default Port=8080, got %s", cfg.Port)
	}
	if cfg.Env != "local" {
		t.Errorf("expected default Env=local, got %s", cfg.Env)
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	// Clear all environment variables
	cleanupEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error when DATABASE_URL is missing")
	}

	expectedError := "DATABASE_URL environment variable is required"
	if err.Error() != expectedError {
		t.Errorf("expected error message=%q, got %q", expectedError, err.Error())
	}
}

func TestLoad_EmptyDatabaseURL(t *testing.T) {
	// Set DATABASE_URL to empty string
	os.Setenv("DATABASE_URL", "")
	defer cleanupEnv()

	_, err := Load()
	if err == nil {
		t.Fatal("Load() should return error when DATABASE_URL is empty")
	}
}

// cleanupEnv removes test environment variables
func cleanupEnv() {
	os.Unsetenv("PORT")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("ENV")
}
