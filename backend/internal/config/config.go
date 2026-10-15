// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the runtime settings for the backend service.
type Config struct {
	Port           string
	DatabaseURL    string
	ReservationTTL time.Duration
	SweepInterval  time.Duration
	DBMaxConns     int32
}

const (
	defaultPort           = "8080"
	defaultDatabaseURL    = "postgres://indico:indico@localhost:5432/indico?sslmode=disable"
	defaultReservationTTL = 5 * time.Minute
	defaultSweepInterval  = 30 * time.Second
	defaultDBMaxConns     = 10
)

// Load reads configuration from the environment, applying defaults for any
// variable that is unset or empty. It returns an error if a value is present
// but cannot be parsed, so misconfiguration fails fast at startup.
func Load() (Config, error) {
	cfg := Config{
		Port:        getString("PORT", defaultPort),
		DatabaseURL: getString("DATABASE_URL", defaultDatabaseURL),
	}

	ttl, err := getDuration("RESERVATION_TTL", defaultReservationTTL)
	if err != nil {
		return Config{}, err
	}
	cfg.ReservationTTL = ttl

	sweep, err := getDuration("SWEEP_INTERVAL", defaultSweepInterval)
	if err != nil {
		return Config{}, err
	}
	cfg.SweepInterval = sweep

	maxConns, err := getInt32("DB_MAX_CONNS", defaultDBMaxConns)
	if err != nil {
		return Config{}, err
	}
	cfg.DBMaxConns = maxConns

	return cfg, nil
}

func getString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s %q: %w", key, v, err)
	}
	return d, nil
}

func getInt32(key string, fallback int32) (int32, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("config: invalid %s %q: %w", key, v, err)
	}
	return int32(n), nil
}
