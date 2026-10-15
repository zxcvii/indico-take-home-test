//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestReserve_Success(t *testing.T) {
	itemID := seedItem(t, 100)

	status, body := api.post(t, "/api/v1/inventory/reserve", map[string]any{
		"user_id":  "usr_e2e",
		"item_id":  itemID,
		"quantity": 2,
	})
	requireStatus(t, status, http.StatusCreated, body)

	res := decode[reserveResponse](t, body)
	if res.Status != "success" {
		t.Fatalf("status field: got %q, want success", res.Status)
	}
	if res.ReservationID == "" {
		t.Fatal("reservation_id is empty")
	}
	if res.ItemID != itemID {
		t.Fatalf("item_id: got %q, want %q", res.ItemID, itemID)
	}
	if res.Quantity != 2 {
		t.Fatalf("quantity: got %d, want 2", res.Quantity)
	}
	if !res.ExpiresAt.After(time.Now()) {
		t.Fatalf("expires_at %s is not in the future", res.ExpiresAt)
	}
	if res.ExpiresAt.After(time.Now().Add(30 * time.Minute)) {
		t.Fatalf("expires_at %s is implausibly far in the future", res.ExpiresAt)
	}
}

func TestReserve_InvalidInput(t *testing.T) {
	itemID := seedItem(t, 10)

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"missing user_id", map[string]any{"item_id": itemID, "quantity": 1}},
		{"missing item_id", map[string]any{"user_id": "usr_e2e", "quantity": 1}},
		{"missing quantity", map[string]any{"user_id": "usr_e2e", "item_id": itemID}},
		{"zero quantity", map[string]any{"user_id": "usr_e2e", "item_id": itemID, "quantity": 0}},
		{"negative quantity", map[string]any{"user_id": "usr_e2e", "item_id": itemID, "quantity": -1}},
		{"empty user_id", map[string]any{"user_id": "", "item_id": itemID, "quantity": 1}},
		{"empty item_id", map[string]any{"user_id": "usr_e2e", "item_id": "", "quantity": 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := api.post(t, "/api/v1/inventory/reserve", tc.payload)
			requireStatus(t, status, http.StatusBadRequest, body)
			requireErrorCode(t, body, "INVALID_INPUT")
		})
	}

	t.Run("malformed json", func(t *testing.T) {
		status, body := api.postRaw(t, "/api/v1/inventory/reserve", "application/json", "{not json")
		requireStatus(t, status, http.StatusBadRequest, body)
		requireErrorCode(t, body, "INVALID_INPUT")
	})
}

func TestReserve_ItemNotFound(t *testing.T) {
	status, body := api.post(t, "/api/v1/inventory/reserve", map[string]any{
		"user_id":  "usr_e2e",
		"item_id":  "item_missing_" + randomSuffix(),
		"quantity": 1,
	})
	requireStatus(t, status, http.StatusNotFound, body)
	requireErrorCode(t, body, "ITEM_NOT_FOUND")
}

func TestReserve_InsufficientStock(t *testing.T) {
	itemID := seedItem(t, 3)

	status, body := api.post(t, "/api/v1/inventory/reserve", map[string]any{
		"user_id":  "usr_e2e",
		"item_id":  itemID,
		"quantity": 5,
	})
	requireStatus(t, status, http.StatusConflict, body)
	requireErrorCode(t, body, "INSUFFICIENT_STOCK")
}
