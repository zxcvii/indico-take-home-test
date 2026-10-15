//go:build e2e

package e2e

import (
	"context"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultBaseURL = "http://localhost:8080"
	defaultDSN     = "postgres://indico:indico@localhost:5432/indico?sslmode=disable"
)

var (
	db  *pgxpool.Pool
	api apiClient
)

func TestMain(m *testing.M) {
	baseURL := envOr("E2E_BASE_URL", defaultBaseURL)
	dsn := envOr("E2E_DATABASE_URL", defaultDSN)
	api = apiClient{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}

	waitForHealth(baseURL)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("e2e: open fixture database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("e2e: fixture database unreachable at %s: %v", dsn, err)
	}
	db = pool
	defer db.Close()

	os.Exit(m.Run())
}

func TestHealth(t *testing.T) {
	status, body := api.get(t, "/health")
	requireStatus(t, status, http.StatusOK, body)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func waitForHealth(baseURL string) {
	deadline := time.Now().Add(20 * time.Second)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Fatalf("e2e: server not healthy at %s; start it with `docker compose up`", baseURL)
}
