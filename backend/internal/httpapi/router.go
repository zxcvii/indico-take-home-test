// Package httpapi is the HTTP layer: router, middleware, handlers and error
// mapping. It is the only package aware of gin.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/zxcvii/indico-take-home-test/backend/internal/service"
)

// HealthChecker reports whether a dependency (the database) is reachable.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Logger    *slog.Logger
	Inventory *service.Inventory
	Health    HealthChecker
}

// New builds the gin engine with middleware and all routes registered.
func New(deps Deps) *gin.Engine {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(requestIDMiddleware())
	r.Use(recoveryMiddleware(logger))
	r.Use(loggingMiddleware(logger))

	r.GET("/health", healthHandler(deps.Health))

	h := &handler{inventory: deps.Inventory}
	api := r.Group("/api/v1/inventory")
	api.POST("/reserve", h.reserve)
	api.POST("/confirm", h.confirm)
	api.GET("/stock", h.stock)

	return r
}

func healthHandler(checker HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker != nil {
			if err := checker.Ping(c.Request.Context()); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
