// Package config loads runtime configuration from environment variables.
package config

import "os"

// Config holds the runtime settings for the backend service.
type Config struct {
	Port        string
	DatabaseURL string
}

const (
	defaultPort        = "8080"
	defaultDatabaseURL = "postgres://indico:indico@localhost:5432/indico?sslmode=disable"
)

// Load reads configuration from the environment, applying defaults for any
// variable that is unset or empty.
func Load() Config {
	return Config{
		Port:        getString("PORT", defaultPort),
		DatabaseURL: getString("DATABASE_URL", defaultDatabaseURL),
	}
}

func getString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
