// Package id generates the opaque, prefixed identifiers used by the API
// (for example "res_883291").
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// randomBytes is the number of random bytes per identifier (16 hex chars).
const randomBytes = 8

// New returns a new identifier with the given prefix.
func New(prefix string) string {
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		panic("id: crypto/rand failed: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// NewReservationID returns a new reservation identifier ("res_...").
func NewReservationID() string { return New("res") }
