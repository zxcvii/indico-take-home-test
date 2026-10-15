//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestLifecycle_ReserveThenConfirmDecrementsTotal(t *testing.T) {
	itemID := seedItem(t, 50)

	assertStock(t, itemID, stockResponse{ItemID: itemID, TotalStock: 50, ReservedStock: 0, AvailableStock: 50})

	status, body := api.post(t, "/api/v1/inventory/reserve", map[string]any{
		"user_id":  "usr_e2e",
		"item_id":  itemID,
		"quantity": 7,
	})
	requireStatus(t, status, http.StatusCreated, body)
	res := decode[reserveResponse](t, body)

	assertStock(t, itemID, stockResponse{ItemID: itemID, TotalStock: 50, ReservedStock: 7, AvailableStock: 43})

	status, body = api.post(t, "/api/v1/inventory/confirm", map[string]any{"reservation_id": res.ReservationID})
	requireStatus(t, status, http.StatusOK, body)

	assertStock(t, itemID, stockResponse{ItemID: itemID, TotalStock: 43, ReservedStock: 0, AvailableStock: 43})
}
