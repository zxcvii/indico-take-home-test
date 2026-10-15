// Package service orchestrates transactions and business rules: handlers never
// touch SQL and the repository never knows about HTTP.
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zxcvii/indico-take-home-test/backend/internal/domain"
	"github.com/zxcvii/indico-take-home-test/backend/internal/id"
	"github.com/zxcvii/indico-take-home-test/backend/internal/repository"
)

// Repository is the persistence contract the service depends on.
type Repository interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Querier() repository.Querier
	Ping(ctx context.Context) error

	ReserveStock(ctx context.Context, q repository.Querier, itemID string, quantity int32) (int64, error)
	ItemExists(ctx context.Context, q repository.Querier, itemID string) (bool, error)
	InsertReservation(ctx context.Context, q repository.Querier, r domain.Reservation, ttlSeconds float64) (time.Time, error)
	ConfirmReservation(ctx context.Context, q repository.Querier, reservationID string) (domain.Reservation, bool, error)
	GetReservation(ctx context.Context, q repository.Querier, reservationID string) (domain.Reservation, error)
	CommitStock(ctx context.Context, q repository.Querier, itemID string, quantity int32) error
	GetItem(ctx context.Context, q repository.Querier, itemID string) (domain.Item, error)
	ExpireDue(ctx context.Context, q repository.Querier) (int, error)
}

// Inventory implements the reservation use cases.
type Inventory struct {
	repo Repository
	ttl  time.Duration
}

// New constructs an Inventory service.
func New(repo Repository, ttl time.Duration) *Inventory {
	return &Inventory{repo: repo, ttl: ttl}
}

// Reserve atomically moves the requested quantity from available into reserved
// stock and creates an ACTIVE reservation. The availability check and the
// increment happen in one conditional UPDATE, so concurrent callers can never
// over-sell.
func (s *Inventory) Reserve(ctx context.Context, in domain.ReserveInput) (domain.Reservation, error) {
	if strings.TrimSpace(in.UserID) == "" {
		return domain.Reservation{}, domain.InvalidInput("user_id is required.", map[string]any{"field": "user_id"})
	}
	if strings.TrimSpace(in.ItemID) == "" {
		return domain.Reservation{}, domain.InvalidInput("item_id is required.", map[string]any{"field": "item_id"})
	}
	if in.Quantity < 1 {
		return domain.Reservation{}, domain.InvalidInput("quantity must be greater than 0.", map[string]any{"field": "quantity"})
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	affected, err := s.repo.ReserveStock(ctx, tx, in.ItemID, in.Quantity)
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	if affected == 0 {
		return domain.Reservation{}, s.reserveFailure(ctx, tx, in)
	}

	reservation := domain.Reservation{
		ReservationID: id.NewReservationID(),
		UserID:        in.UserID,
		ItemID:        in.ItemID,
		Quantity:      in.Quantity,
		Status:        domain.StatusActive,
	}
	expiresAt, err := s.repo.InsertReservation(ctx, tx, reservation, s.ttl.Seconds())
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	reservation.ExpiresAt = expiresAt

	if err := tx.Commit(ctx); err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	return reservation, nil
}

// Confirm commits an ACTIVE, unexpired reservation, decrementing both counters
// exactly once. The status guard makes concurrent or repeated confirms
// idempotent.
func (s *Inventory) Confirm(ctx context.Context, in domain.ConfirmInput) (domain.Reservation, error) {
	if strings.TrimSpace(in.ReservationID) == "" {
		return domain.Reservation{}, domain.InvalidInput("reservation_id is required.", map[string]any{"field": "reservation_id"})
	}

	tx, err := s.repo.Begin(ctx)
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	confirmed, ok, err := s.repo.ConfirmReservation(ctx, tx, in.ReservationID)
	if err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	if !ok {
		return domain.Reservation{}, s.confirmFailure(ctx, tx, in.ReservationID)
	}

	if err := s.repo.CommitStock(ctx, tx, confirmed.ItemID, confirmed.Quantity); err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Reservation{}, domain.Internal(err)
	}
	return confirmed, nil
}

// GetStock returns the live stock breakdown for an item.
func (s *Inventory) GetStock(ctx context.Context, itemID string) (domain.Stock, error) {
	if strings.TrimSpace(itemID) == "" {
		return domain.Stock{}, domain.InvalidInput("item_id is required.", map[string]any{"field": "item_id"})
	}

	item, err := s.repo.GetItem(ctx, s.repo.Querier(), itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Stock{}, domain.ItemNotFound(itemID)
	}
	if err != nil {
		return domain.Stock{}, domain.Internal(err)
	}

	return domain.Stock{
		ItemID:         item.ItemID,
		TotalStock:     item.TotalStock,
		ReservedStock:  item.ReservedStock,
		AvailableStock: item.AvailableStock(),
	}, nil
}

// ExpireDue releases every reservation past its TTL and returns how many were
// expired. It is driven by the sweeper.
func (s *Inventory) ExpireDue(ctx context.Context) (int, error) {
	expired, err := s.repo.ExpireDue(ctx, s.repo.Querier())
	if err != nil {
		return 0, domain.Internal(err)
	}
	return expired, nil
}

// reserveFailure disambiguates a zero-rows reserve UPDATE into either a missing
// item or insufficient stock.
func (s *Inventory) reserveFailure(ctx context.Context, q repository.Querier, in domain.ReserveInput) error {
	exists, err := s.repo.ItemExists(ctx, q, in.ItemID)
	if err != nil {
		return domain.Internal(err)
	}
	if !exists {
		return domain.ItemNotFound(in.ItemID)
	}

	item, err := s.repo.GetItem(ctx, q, in.ItemID)
	if err != nil {
		return domain.Internal(err)
	}
	return domain.InsufficientStock(in.ItemID, item.AvailableStock(), in.Quantity)
}

// confirmFailure inspects why a confirm matched no rows: missing, expired or
// already confirmed.
func (s *Inventory) confirmFailure(ctx context.Context, q repository.Querier, reservationID string) error {
	reservation, err := s.repo.GetReservation(ctx, q, reservationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReservationNotFound(reservationID)
	}
	if err != nil {
		return domain.Internal(err)
	}

	switch {
	case reservation.Status == domain.StatusConfirmed:
		return domain.AlreadyConfirmed(reservationID)
	case reservation.Status == domain.StatusExpired, !reservation.ExpiresAt.After(time.Now()):
		return domain.ReservationExpired(reservationID, reservation.ExpiresAt)
	default:
		return domain.Internal(errors.New("reservation is in an unexpected state"))
	}
}
