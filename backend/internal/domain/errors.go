package domain

import (
	"fmt"
	"time"
)

// ErrorCode is a stable, machine-readable error discriminator. The values match
// the `error.code` enum in openapi.yaml.
type ErrorCode string

const (
	CodeInvalidInput        ErrorCode = "INVALID_INPUT"
	CodeItemNotFound        ErrorCode = "ITEM_NOT_FOUND"
	CodeInsufficientStock   ErrorCode = "INSUFFICIENT_STOCK"
	CodeReservationNotFound ErrorCode = "RESERVATION_NOT_FOUND"
	CodeReservationExpired  ErrorCode = "RESERVATION_EXPIRED"
	CodeAlreadyConfirmed    ErrorCode = "ALREADY_CONFIRMED"
	CodeInternal            ErrorCode = "INTERNAL_ERROR"
)

// Error is a typed, API-aware error carrying a stable Code, a safe
// human-readable Message, optional structured Details, and an optional wrapped
// cause that is logged but never serialised to clients.
type Error struct {
	Code    ErrorCode
	Message string
	Details map[string]any
	cause   error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return string(e.Code) + ": " + e.Message + ": " + e.cause.Error()
	}
	return string(e.Code) + ": " + e.Message
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.cause }

// WithCause attaches an underlying error to the typed error.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

// NewError builds a typed *Error.
func NewError(code ErrorCode, message string, details map[string]any) *Error {
	return &Error{Code: code, Message: message, Details: details}
}

// InvalidInput reports a malformed, missing or out-of-range request field.
func InvalidInput(message string, details map[string]any) *Error {
	return NewError(CodeInvalidInput, message, details)
}

// ItemNotFound reports an unknown item_id.
func ItemNotFound(itemID string) *Error {
	return NewError(CodeItemNotFound, "Item '"+itemID+"' does not exist.", map[string]any{
		"item_id": itemID,
	})
}

// InsufficientStock reports that fewer units are available than requested.
func InsufficientStock(itemID string, available, requested int32) *Error {
	return NewError(
		CodeInsufficientStock,
		fmt.Sprintf("Only %d units available, requested %d.", available, requested),
		map[string]any{
			"item_id":   itemID,
			"available": available,
			"requested": requested,
		},
	)
}

// ReservationNotFound reports an unknown reservation_id.
func ReservationNotFound(reservationID string) *Error {
	return NewError(CodeReservationNotFound, "Reservation '"+reservationID+"' does not exist.", map[string]any{
		"reservation_id": reservationID,
	})
}

// ReservationExpired reports a reservation that can no longer be confirmed.
func ReservationExpired(reservationID string, expiresAt time.Time) *Error {
	return NewError(
		CodeReservationExpired,
		fmt.Sprintf("Reservation '%s' expired at %s.", reservationID, expiresAt.UTC().Format(time.RFC3339)),
		map[string]any{
			"reservation_id": reservationID,
			"expires_at":     expiresAt.UTC().Format(time.RFC3339),
		},
	)
}

// AlreadyConfirmed reports a repeat confirm attempt on a confirmed reservation.
func AlreadyConfirmed(reservationID string) *Error {
	return NewError(CodeAlreadyConfirmed, "Reservation '"+reservationID+"' has already been confirmed.", map[string]any{
		"reservation_id": reservationID,
	})
}

// Internal wraps an unexpected fault as a generic, client-safe error.
func Internal(cause error) *Error {
	return NewError(CodeInternal, "An unexpected error occurred.", nil).WithCause(cause)
}
