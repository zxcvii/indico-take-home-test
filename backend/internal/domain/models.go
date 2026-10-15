// Package domain holds the core business types and typed errors shared by the
// service, repository and HTTP layers. It depends on neither pgx nor gin.
package domain

import "time"

// ReservationStatus is the lifecycle state of a reservation, mirroring the
// PostgreSQL `reservation_status` enum. The lifecycle is strictly linear: a
// reservation is ACTIVE until it is either CONFIRMED or EXPIRED, and never
// moves again after that.
type ReservationStatus string

const (
	StatusActive    ReservationStatus = "ACTIVE"
	StatusConfirmed ReservationStatus = "CONFIRMED"
	StatusExpired   ReservationStatus = "EXPIRED"
)

// Item is a sellable product and its denormalised stock counters.
// AvailableStock is always derived, never persisted.
type Item struct {
	ItemID        string
	Name          string
	TotalStock    int32
	ReservedStock int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AvailableStock returns the units that can still be reserved.
func (i Item) AvailableStock() int32 {
	return i.TotalStock - i.ReservedStock
}

// Reservation is a hold placed by a user against a single item.
type Reservation struct {
	ReservationID string
	UserID        string
	ItemID        string
	Quantity      int32
	Status        ReservationStatus
	CreatedAt     time.Time
	ExpiresAt     time.Time
	ConfirmedAt   *time.Time
}

// Stock is the read model rendered by the stock endpoint.
type Stock struct {
	ItemID         string
	TotalStock     int32
	ReservedStock  int32
	AvailableStock int32
}

// ReserveInput is the validated command for creating a reservation.
type ReserveInput struct {
	UserID   string
	ItemID   string
	Quantity int32
}

// ConfirmInput is the validated command for confirming a reservation.
type ConfirmInput struct {
	ReservationID string
}
