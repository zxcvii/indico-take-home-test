//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func seedItem(t *testing.T, totalStock int) string {
	t.Helper()

	itemID := "item_e2e_" + randomSuffix()
	_, err := db.Exec(context.Background(),
		`INSERT INTO items (item_id, name, total_stock) VALUES ($1, $2, $3)`,
		itemID, "E2E Fixture Item", totalStock)
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.Exec(ctx, `DELETE FROM reservations WHERE item_id = $1`, itemID)
		_, _ = db.Exec(ctx, `DELETE FROM items WHERE item_id = $1`, itemID)
	})
	return itemID
}

func forceExpire(t *testing.T, reservationID string) {
	t.Helper()

	if _, err := db.Exec(context.Background(),
		`UPDATE reservations SET expires_at = now() - interval '1 second' WHERE reservation_id = $1`,
		reservationID); err != nil {
		t.Fatalf("force expire reservation: %v", err)
	}
}

func randomSuffix() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
