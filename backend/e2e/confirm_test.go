//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestConfirm_Success(t *testing.T) {
	res := reserveOne(t, 10)

	status, body := api.post(t, "/api/v1/inventory/confirm", map[string]any{
		"reservation_id": res.ReservationID,
	})
	requireStatus(t, status, http.StatusOK, body)

	conf := decode[confirmResponse](t, body)
	if conf.Status != "success" {
		t.Fatalf("status field: got %q, want success", conf.Status)
	}
	if conf.ReservationID != res.ReservationID {
		t.Fatalf("reservation_id: got %q, want %q", conf.ReservationID, res.ReservationID)
	}
	if conf.ConfirmedAt.IsZero() {
		t.Fatal("confirmed_at is zero")
	}
}

func TestConfirm_NotFound(t *testing.T) {
	status, body := api.post(t, "/api/v1/inventory/confirm", map[string]any{
		"reservation_id": "res_missing_" + randomSuffix(),
	})
	requireStatus(t, status, http.StatusNotFound, body)
	requireErrorCode(t, body, "RESERVATION_NOT_FOUND")
}

func TestConfirm_AlreadyConfirmed(t *testing.T) {
	res := reserveOne(t, 10)

	status, body := api.post(t, "/api/v1/inventory/confirm", map[string]any{"reservation_id": res.ReservationID})
	requireStatus(t, status, http.StatusOK, body)

	status, body = api.post(t, "/api/v1/inventory/confirm", map[string]any{"reservation_id": res.ReservationID})
	requireStatus(t, status, http.StatusConflict, body)
	requireErrorCode(t, body, "ALREADY_CONFIRMED")
}

func TestConfirm_Expired(t *testing.T) {
	res := reserveOne(t, 10)
	forceExpire(t, res.ReservationID)

	status, body := api.post(t, "/api/v1/inventory/confirm", map[string]any{
		"reservation_id": res.ReservationID,
	})
	requireStatus(t, status, http.StatusGone, body)
	requireErrorCode(t, body, "RESERVATION_EXPIRED")
}
