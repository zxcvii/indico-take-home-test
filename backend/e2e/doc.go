// Package e2e holds black-box end-to-end tests for the HTTP API described in
// openapi.yaml. They run against a live server and a real PostgreSQL.
//
// The suite is opt-in and excluded from the default build. Run it with:
//
//	docker compose up -d
//	go test -tags=e2e -v ./e2e/...
//
// Overridable environment variables:
//
//	E2E_BASE_URL      base URL of the running server (default http://localhost:8080)
//	E2E_DATABASE_URL  DSN used only to seed and clean up test fixtures
package e2e
