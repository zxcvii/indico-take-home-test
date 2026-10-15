package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zxcvii/indico-take-home-test/backend/internal/domain"
	"github.com/zxcvii/indico-take-home-test/backend/internal/id"
)

const requestIDHeader = "X-Request-ID"

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    domain.ErrorCode `json:"code"`
	Message string           `json:"message"`
	Details map[string]any   `json:"details,omitempty"`
}

func statusForCode(code domain.ErrorCode) int {
	switch code {
	case domain.CodeInvalidInput:
		return http.StatusBadRequest
	case domain.CodeItemNotFound, domain.CodeReservationNotFound:
		return http.StatusNotFound
	case domain.CodeInsufficientStock, domain.CodeAlreadyConfirmed:
		return http.StatusConflict
	case domain.CodeReservationExpired:
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}

// AbortWithError writes the standard error envelope and stops the handler chain.
// A *domain.Error maps to its registered status and code; any other error is
// reported as a generic INTERNAL_ERROR so no internal detail reaches the client.
func AbortWithError(c *gin.Context, err error) {
	var domErr *domain.Error
	if !errors.As(err, &domErr) {
		domErr = domain.Internal(err)
	}

	if cause := domErr.Unwrap(); cause != nil {
		_ = c.Error(cause)
	}

	c.AbortWithStatusJSON(statusForCode(domErr.Code), errorEnvelope{
		Error: errorBody{
			Code:    domErr.Code,
			Message: domErr.Message,
			Details: domErr.Details,
		},
	})
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := id.New("req")
		c.Set("request_id", requestID)
		c.Header(requestIDHeader, requestID)
		c.Next()
	}
}

func recoveryMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"error", fmt.Sprint(r),
					"path", c.Request.URL.Path,
					"request_id", c.GetString("request_id"),
				)
				AbortWithError(c, fmt.Errorf("panic: %v", r))
			}
		}()
		c.Next()
	}
}

func loggingMiddleware(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString("request_id"),
		}
		for _, e := range c.Errors {
			attrs = append(attrs, "cause", e.Error())
		}

		switch {
		case c.Writer.Status() >= http.StatusInternalServerError:
			logger.Error("request", attrs...)
		case c.Writer.Status() >= http.StatusBadRequest:
			logger.Warn("request", attrs...)
		default:
			logger.Info("request", attrs...)
		}
	}
}
