//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"testing"
)

func reserveOne(t *testing.T, totalStock int) reserveResponse {
	t.Helper()

	itemID := seedItem(t, totalStock)
	status, body := api.post(t, "/api/v1/inventory/reserve", map[string]any{
		"user_id":  "usr_e2e",
		"item_id":  itemID,
		"quantity": 1,
	})
	requireStatus(t, status, http.StatusCreated, body)
	return decode[reserveResponse](t, body)
}

func getStock(t *testing.T, itemID string) stockResponse {
	t.Helper()

	status, body := api.get(t, "/api/v1/inventory/stock?item_id="+url.QueryEscape(itemID))
	requireStatus(t, status, http.StatusOK, body)
	return decode[stockResponse](t, body)
}

func assertStock(t *testing.T, itemID string, want stockResponse) {
	t.Helper()

	got := getStock(t, itemID)
	if got != want {
		t.Fatalf("stock mismatch:\n got:  %+v\n want: %+v", got, want)
	}
}
