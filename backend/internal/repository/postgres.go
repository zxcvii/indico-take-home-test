// Package repository owns all SQL access. No business rules live here and no
// HTTP types leak in.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zxcvii/indico-take-home-test/backend/internal/domain"
)

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting the service
// run the same statements inside or outside a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store is the PostgreSQL-backed repository.
type Store struct {
	pool *pgxpool.Pool
}

// NewPool opens a pgx connection pool bounded to maxConns (ignored when <= 0)
// and verifies connectivity with a ping before returning.
func NewPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("repository: parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("repository: open pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("repository: ping: %w", err)
	}

	return pool, nil
}

// NewStore wraps an existing pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Querier returns the pool for statements that do not need a transaction.
func (s *Store) Querier() Querier {
	return s.pool
}

// Begin starts a transaction.
func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// Close releases all pooled connections.
func (s *Store) Close() {
	s.pool.Close()
}

const reserveStockSQL = `
UPDATE items
   SET reserved_stock = reserved_stock + $1,
       updated_at     = now()
 WHERE item_id = $2
   AND (total_stock - reserved_stock) >= $1`

// ReserveStock atomically increments reserved_stock when enough availability
// exists, returning the number of rows affected (0 means unknown item or
// insufficient stock).
func (s *Store) ReserveStock(ctx context.Context, q Querier, itemID string, quantity int32) (int64, error) {
	tag, err := q.Exec(ctx, reserveStockSQL, quantity, itemID)
	if err != nil {
		return 0, fmt.Errorf("repository: reserve stock: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ItemExists reports whether an item row exists.
func (s *Store) ItemExists(ctx context.Context, q Querier, itemID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM items WHERE item_id = $1)`, itemID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("repository: item exists: %w", err)
	}
	return exists, nil
}

const insertReservationSQL = `
INSERT INTO reservations (reservation_id, user_id, item_id, quantity, status, expires_at)
VALUES ($1, $2, $3, $4, 'ACTIVE', now() + make_interval(secs => $5))
RETURNING expires_at`

// InsertReservation persists a new ACTIVE reservation and returns its
// database-assigned expiry.
func (s *Store) InsertReservation(ctx context.Context, q Querier, r domain.Reservation, ttlSeconds float64) (time.Time, error) {
	var expiresAt time.Time
	err := q.QueryRow(ctx, insertReservationSQL, r.ReservationID, r.UserID, r.ItemID, r.Quantity, ttlSeconds).Scan(&expiresAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("repository: insert reservation: %w", err)
	}
	return expiresAt, nil
}

const confirmReservationSQL = `
UPDATE reservations
   SET status = 'CONFIRMED', confirmed_at = now()
 WHERE reservation_id = $1
   AND status = 'ACTIVE'
   AND expires_at > now()
 RETURNING item_id, quantity, confirmed_at`

// ConfirmReservation flips an ACTIVE, unexpired reservation to CONFIRMED. The
// boolean is false when no row matched (missing, expired or already confirmed).
func (s *Store) ConfirmReservation(ctx context.Context, q Querier, reservationID string) (domain.Reservation, bool, error) {
	var r domain.Reservation
	var confirmedAt time.Time
	err := q.QueryRow(ctx, confirmReservationSQL, reservationID).Scan(&r.ItemID, &r.Quantity, &confirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Reservation{}, false, nil
	}
	if err != nil {
		return domain.Reservation{}, false, fmt.Errorf("repository: confirm reservation: %w", err)
	}
	r.ReservationID = reservationID
	r.Status = domain.StatusConfirmed
	r.ConfirmedAt = &confirmedAt
	return r, true, nil
}

const getReservationSQL = `
SELECT reservation_id, user_id, item_id, quantity, status::text, created_at, expires_at, confirmed_at
  FROM reservations
 WHERE reservation_id = $1`

// GetReservation loads a reservation, returning pgx.ErrNoRows when absent.
func (s *Store) GetReservation(ctx context.Context, q Querier, reservationID string) (domain.Reservation, error) {
	var r domain.Reservation
	var status string
	err := q.QueryRow(ctx, getReservationSQL, reservationID).
		Scan(&r.ReservationID, &r.UserID, &r.ItemID, &r.Quantity, &status, &r.CreatedAt, &r.ExpiresAt, &r.ConfirmedAt)
	if err != nil {
		return domain.Reservation{}, err
	}
	r.Status = domain.ReservationStatus(status)
	return r, nil
}

const commitStockSQL = `
UPDATE items
   SET total_stock    = total_stock - $1,
       reserved_stock = reserved_stock - $1,
       updated_at     = now()
 WHERE item_id = $2`

// CommitStock removes confirmed units from both counters.
func (s *Store) CommitStock(ctx context.Context, q Querier, itemID string, quantity int32) error {
	if _, err := q.Exec(ctx, commitStockSQL, quantity, itemID); err != nil {
		return fmt.Errorf("repository: commit stock: %w", err)
	}
	return nil
}

const getItemSQL = `
SELECT item_id, name, total_stock, reserved_stock, created_at, updated_at
  FROM items
 WHERE item_id = $1`

// GetItem loads an item, returning pgx.ErrNoRows when absent.
func (s *Store) GetItem(ctx context.Context, q Querier, itemID string) (domain.Item, error) {
	var it domain.Item
	err := q.QueryRow(ctx, getItemSQL, itemID).
		Scan(&it.ItemID, &it.Name, &it.TotalStock, &it.ReservedStock, &it.CreatedAt, &it.UpdatedAt)
	if err != nil {
		return domain.Item{}, err
	}
	return it, nil
}

const expireDueSQL = `
WITH due AS (
    SELECT reservation_id, item_id, quantity
      FROM reservations
     WHERE status = 'ACTIVE' AND expires_at <= now()
     FOR UPDATE SKIP LOCKED
),
expired AS (
    UPDATE reservations r
       SET status = 'EXPIRED'
      FROM due
     WHERE r.reservation_id = due.reservation_id
    RETURNING due.item_id AS item_id, due.quantity AS quantity
),
released AS (
    UPDATE items i
       SET reserved_stock = i.reserved_stock - agg.quantity,
           updated_at     = now()
      FROM (
            SELECT item_id, SUM(quantity)::int AS quantity
              FROM expired
             GROUP BY item_id
           ) agg
     WHERE i.item_id = agg.item_id
    RETURNING i.item_id
)
SELECT count(*) FROM expired`

// ExpireDue marks every ACTIVE reservation past its expiry as EXPIRED and
// returns the held quantity to available stock, in a single statement. FOR
// UPDATE SKIP LOCKED keeps it safe under overlapping sweeps. It returns the
// number of reservations expired.
func (s *Store) ExpireDue(ctx context.Context, q Querier) (int, error) {
	var expired int
	if err := q.QueryRow(ctx, expireDueSQL).Scan(&expired); err != nil {
		return 0, fmt.Errorf("repository: expire due reservations: %w", err)
	}
	return expired, nil
}
