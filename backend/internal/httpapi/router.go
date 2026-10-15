// Package httpapi is the HTTP layer.
package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthChecker reports whether a dependency (the database) is reachable.
type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the HTTP layer needs.
type Deps struct {
	Health HealthChecker
}

// New builds the gin engine. Only the health endpoint is registered for now.
func New(deps Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.GET("/health", healthHandler(deps.Health))
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
