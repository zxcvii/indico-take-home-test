// Command server is the entrypoint for the inventory reservation backend.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zxcvii/indico-take-home-test/backend/internal/config"
	"github.com/zxcvii/indico-take-home-test/backend/internal/httpapi"
	"github.com/zxcvii/indico-take-home-test/backend/internal/repository"
	"github.com/zxcvii/indico-take-home-test/backend/internal/service"
	"github.com/zxcvii/indico-take-home-test/backend/internal/sweeper"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := repository.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	store := repository.NewStore(pool)
	defer store.Close()

	inventory := service.New(store, cfg.ReservationTTL)

	sweepCtx, stopSweeper := context.WithCancel(context.Background())
	defer stopSweeper()
	expiry := sweeper.New(cfg.SweepInterval, func(sweepCtx context.Context) error {
		expired, err := inventory.ExpireDue(sweepCtx)
		if err != nil {
			return err
		}
		if expired > 0 {
			logger.Info("expired reservations released", "count", expired)
		}
		return nil
	}, logger)
	go expiry.Run(sweepCtx)

	router := httpapi.New(httpapi.Deps{
		Logger:    logger,
		Inventory: inventory,
		Health:    store,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	stop()

	// Drain in-flight requests first; the deferred sweeper stop and pool close
	// then run in the intended order (stop sweeper, close pool) on return.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}
