package service_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zxcvii/indico-take-home-test/backend/internal/domain"
	"github.com/zxcvii/indico-take-home-test/backend/internal/repository"
	"github.com/zxcvii/indico-take-home-test/backend/internal/service"
)

const defaultTestDSN = "postgres://indico:indico@localhost:5432/indico?sslmode=disable"

var (
	testPool *pgxpool.Pool
	testSkip string
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err == nil {
		if err = pool.Ping(ctx); err == nil {
			var ready bool
			err = pool.QueryRow(ctx, `SELECT to_regclass('public.items') IS NOT NULL`).Scan(&ready)
			if err == nil && !ready {
				err = errors.New("schema not found; apply backend/migrations (for example `docker compose up`)")
			}
		}
	}
	if err != nil {
		testSkip = err.Error()
		pool = nil
	}
	testPool = pool

	code := m.Run()
	if testPool != nil {
		testPool.Close()
	}
	os.Exit(code)
}

func newService(t *testing.T) *service.Inventory {
	t.Helper()
	if testPool == nil {
		t.Skipf("skipping: %s", testSkip)
	}
	return service.New(repository.NewStore(testPool), 5*time.Minute)
}

func seedItem(t *testing.T, totalStock int32) string {
	t.Helper()

	itemID := "item_test_" + randomSuffix()
	if _, err := testPool.Exec(context.Background(),
		`INSERT INTO items (item_id, name, total_stock) VALUES ($1, $2, $3)`,
		itemID, "Test Item", totalStock); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testPool.Exec(ctx, `DELETE FROM reservations WHERE item_id = $1`, itemID)
		_, _ = testPool.Exec(ctx, `DELETE FROM items WHERE item_id = $1`, itemID)
	})
	return itemID
}

func randomSuffix() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func requireCode(t *testing.T, err error, want domain.ErrorCode) {
	t.Helper()

	var domErr *domain.Error
	if !errors.As(err, &domErr) {
		t.Fatalf("error: got %v, want *domain.Error with code %s", err, want)
	}
	if domErr.Code != want {
		t.Fatalf("error code: got %s, want %s", domErr.Code, want)
	}
}

func TestReserve_Success(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 10)

	reservation, err := svc.Reserve(context.Background(), domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 3})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if reservation.ReservationID == "" {
		t.Fatal("reservation_id is empty")
	}
	if reservation.Status != domain.StatusActive {
		t.Fatalf("status: got %s, want ACTIVE", reservation.Status)
	}
	if !reservation.ExpiresAt.After(time.Now()) {
		t.Fatalf("expires_at is not in the future: %s", reservation.ExpiresAt)
	}

	stock := getStock(t, svc, itemID)
	if stock.ReservedStock != 3 || stock.AvailableStock != 7 {
		t.Fatalf("stock after reserve: %+v", stock)
	}
}

func TestReserve_InsufficientStock(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 3)

	_, err := svc.Reserve(context.Background(), domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 5})
	requireCode(t, err, domain.CodeInsufficientStock)

	stock := getStock(t, svc, itemID)
	if stock.ReservedStock != 0 {
		t.Fatalf("reserved_stock changed on failed reserve: %d", stock.ReservedStock)
	}
}

func TestReserve_ItemNotFound(t *testing.T) {
	svc := newService(t)

	_, err := svc.Reserve(context.Background(), domain.ReserveInput{
		UserID: "usr_1", ItemID: "item_missing_" + randomSuffix(), Quantity: 1,
	})
	requireCode(t, err, domain.CodeItemNotFound)
}

func TestReserve_InvalidInput(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 10)

	cases := []struct {
		name  string
		input domain.ReserveInput
	}{
		{"empty user_id", domain.ReserveInput{ItemID: itemID, Quantity: 1}},
		{"empty item_id", domain.ReserveInput{UserID: "usr_1", Quantity: 1}},
		{"zero quantity", domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 0}},
		{"negative quantity", domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: -1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Reserve(context.Background(), tc.input)
			requireCode(t, err, domain.CodeInvalidInput)
		})
	}
}

func TestConfirm_Success(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 10)

	reservation, err := svc.Reserve(context.Background(), domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 4})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}

	confirmed, err := svc.Confirm(context.Background(), domain.ConfirmInput{ReservationID: reservation.ReservationID})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if confirmed.Status != domain.StatusConfirmed || confirmed.ConfirmedAt == nil {
		t.Fatalf("unexpected confirmed reservation: %+v", confirmed)
	}

	stock := getStock(t, svc, itemID)
	if stock.TotalStock != 6 || stock.ReservedStock != 0 || stock.AvailableStock != 6 {
		t.Fatalf("stock after confirm: %+v", stock)
	}
}

func TestConfirm_Expired(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 10)

	reservation, err := svc.Reserve(context.Background(), domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 1})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		`UPDATE reservations SET expires_at = now() - interval '1 second' WHERE reservation_id = $1`,
		reservation.ReservationID); err != nil {
		t.Fatalf("force expire: %v", err)
	}

	_, err = svc.Confirm(context.Background(), domain.ConfirmInput{ReservationID: reservation.ReservationID})
	requireCode(t, err, domain.CodeReservationExpired)
}

func TestConfirm_AlreadyConfirmed(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 10)

	reservation, err := svc.Reserve(context.Background(), domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 1})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), domain.ConfirmInput{ReservationID: reservation.ReservationID}); err != nil {
		t.Fatalf("first confirm: %v", err)
	}

	_, err = svc.Confirm(context.Background(), domain.ConfirmInput{ReservationID: reservation.ReservationID})
	requireCode(t, err, domain.CodeAlreadyConfirmed)

	stock := getStock(t, svc, itemID)
	if stock.TotalStock != 9 {
		t.Fatalf("total_stock decremented twice: %+v", stock)
	}
}

func TestConfirm_NotFound(t *testing.T) {
	svc := newService(t)

	_, err := svc.Confirm(context.Background(), domain.ConfirmInput{ReservationID: "res_missing_" + randomSuffix()})
	requireCode(t, err, domain.CodeReservationNotFound)
}

func TestGetStock_ItemNotFound(t *testing.T) {
	svc := newService(t)

	_, err := svc.GetStock(context.Background(), "item_missing_"+randomSuffix())
	requireCode(t, err, domain.CodeItemNotFound)
}

func TestExpireDue_ReleasesStock(t *testing.T) {
	svc := newService(t)
	itemID := seedItem(t, 10)

	reservation, err := svc.Reserve(context.Background(), domain.ReserveInput{UserID: "usr_1", ItemID: itemID, Quantity: 4})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := testPool.Exec(context.Background(),
		`UPDATE reservations SET expires_at = now() - interval '1 second' WHERE reservation_id = $1`,
		reservation.ReservationID); err != nil {
		t.Fatalf("force expire: %v", err)
	}

	expired, err := svc.ExpireDue(context.Background())
	if err != nil {
		t.Fatalf("expire due: %v", err)
	}
	if expired < 1 {
		t.Fatalf("expired %d reservations, want at least 1", expired)
	}

	stock := getStock(t, svc, itemID)
	if stock.ReservedStock != 0 || stock.AvailableStock != 10 {
		t.Fatalf("stock after expiry: %+v", stock)
	}
}

func TestReserve_ConcurrentNoOversell(t *testing.T) {
	svc := newService(t)

	const (
		totalStock = 50
		attempts   = 200
	)
	itemID := seedItem(t, totalStock)

	var successes, unexpected int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start

			_, err := svc.Reserve(context.Background(), domain.ReserveInput{
				UserID:   fmt.Sprintf("usr_%d", i),
				ItemID:   itemID,
				Quantity: 1,
			})
			if err == nil {
				atomic.AddInt64(&successes, 1)
				return
			}

			var domErr *domain.Error
			if !errors.As(err, &domErr) || domErr.Code != domain.CodeInsufficientStock {
				atomic.AddInt64(&unexpected, 1)
			}
		}(i)
	}

	close(start)
	wg.Wait()

	if unexpected != 0 {
		t.Fatalf("unexpected errors: %d (want 0)", unexpected)
	}
	if successes != totalStock {
		t.Fatalf("successes: got %d, want exactly %d", successes, totalStock)
	}

	stock := getStock(t, svc, itemID)
	if stock.ReservedStock != totalStock || stock.AvailableStock != 0 {
		t.Fatalf("stock after concurrent reserve: %+v", stock)
	}
}

func getStock(t *testing.T, svc *service.Inventory, itemID string) domain.Stock {
	t.Helper()

	stock, err := svc.GetStock(context.Background(), itemID)
	if err != nil {
		t.Fatalf("get stock: %v", err)
	}
	return stock
}
