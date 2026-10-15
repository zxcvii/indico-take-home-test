//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestStock_Success(t *testing.T) {
	itemID := seedItem(t, 100)
	api.post(t, "/api/v1/inventory/reserve", map[string]any{
		"user_id":  "usr_e2e",
		"item_id":  itemID,
		"quantity": 15,
	})

	assertStock(t, itemID, stockResponse{
		ItemID:         itemID,
		TotalStock:     100,
		ReservedStock:  15,
		AvailableStock: 85,
	})
}

func TestStock_ItemNotFound(t *testing.T) {
	status, body := api.get(t, "/api/v1/inventory/stock?item_id=item_missing_"+randomSuffix())
	requireStatus(t, status, http.StatusNotFound, body)
	requireErrorCode(t, body, "ITEM_NOT_FOUND")
}

func TestStock_InvalidInput(t *testing.T) {
	status, body := api.get(t, "/api/v1/inventory/stock")
	requireStatus(t, status, http.StatusBadRequest, body)
	requireErrorCode(t, body, "INVALID_INPUT")
}
