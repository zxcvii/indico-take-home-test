//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReserve_ConcurrentNoOversell(t *testing.T) {
	const (
		totalStock = 50
		attempts   = 200
	)
	itemID := seedItem(t, totalStock)

	var created, unexpected int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := jsonBody(map[string]any{
				"user_id":  fmt.Sprintf("usr_e2e_%d", i),
				"item_id":  itemID,
				"quantity": 1,
			})
			<-start

			status, _, err := api.do(http.MethodPost, "/api/v1/inventory/reserve", "application/json", payload)
			if err != nil {
				atomic.AddInt64(&unexpected, 1)
				return
			}
			switch status {
			case http.StatusCreated:
				atomic.AddInt64(&created, 1)
			case http.StatusConflict:
			default:
				atomic.AddInt64(&unexpected, 1)
			}
		}(i)
	}

	close(start)
	wg.Wait()

	if unexpected != 0 {
		t.Fatalf("unexpected responses: %d (want 0); only 201 and 409 are valid", unexpected)
	}
	if created != totalStock {
		t.Fatalf("created reservations: got %d, want exactly %d", created, totalStock)
	}

	got := getStock(t, itemID)
	if got.ReservedStock != totalStock {
		t.Fatalf("reserved_stock: got %d, want %d (over-sell or under-sell)", got.ReservedStock, totalStock)
	}
	if got.AvailableStock != 0 {
		t.Fatalf("available_stock: got %d, want 0", got.AvailableStock)
	}
	if got.TotalStock != totalStock {
		t.Fatalf("total_stock: got %d, want %d", got.TotalStock, totalStock)
	}
}
