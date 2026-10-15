// Package sweeper runs the background loop that returns timed-out reservations
// to available stock.
package sweeper

import (
	"context"
	"log/slog"
	"time"
)

// SweepFunc performs one expiry pass. Implementations must be safe under
// concurrent execution.
type SweepFunc func(ctx context.Context) error

// Sweeper periodically invokes a SweepFunc until its context is cancelled.
type Sweeper struct {
	interval time.Duration
	sweep    SweepFunc
	logger   *slog.Logger
}

// New builds a Sweeper that runs sweep every interval.
func New(interval time.Duration, sweep SweepFunc, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sweeper{interval: interval, sweep: sweep, logger: logger}
}

// Run blocks until ctx is cancelled, invoking the sweep function on every tick.
// Start it as a goroutine and stop it by cancelling ctx during shutdown.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.Info("expiry sweeper started", "interval", s.interval)
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("expiry sweeper stopped")
			return
		case <-ticker.C:
			if err := s.sweep(ctx); err != nil {
				s.logger.Error("expiry sweep failed", "error", err)
			}
		}
	}
}
