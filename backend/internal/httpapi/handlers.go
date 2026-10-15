package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zxcvii/indico-take-home-test/backend/internal/domain"
	"github.com/zxcvii/indico-take-home-test/backend/internal/service"
)

type handler struct {
	inventory *service.Inventory
}

type reserveRequest struct {
	UserID   string `json:"user_id"`
	ItemID   string `json:"item_id"`
	Quantity int32  `json:"quantity"`
}

type reserveResponse struct {
	Status        string    `json:"status"`
	ReservationID string    `json:"reservation_id"`
	ItemID        string    `json:"item_id"`
	Quantity      int32     `json:"quantity"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (h *handler) reserve(c *gin.Context) {
	var req reserveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		AbortWithError(c, domain.InvalidInput("Request body must be valid JSON with user_id, item_id and quantity.", nil))
		return
	}

	reservation, err := h.inventory.Reserve(c.Request.Context(), domain.ReserveInput{
		UserID:   req.UserID,
		ItemID:   req.ItemID,
		Quantity: req.Quantity,
	})
	if err != nil {
		AbortWithError(c, err)
		return
	}

	c.JSON(http.StatusCreated, reserveResponse{
		Status:        "success",
		ReservationID: reservation.ReservationID,
		ItemID:        reservation.ItemID,
		Quantity:      reservation.Quantity,
		ExpiresAt:     reservation.ExpiresAt,
	})
}

type confirmRequest struct {
	ReservationID string `json:"reservation_id"`
}

type confirmResponse struct {
	Status        string    `json:"status"`
	ReservationID string    `json:"reservation_id"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
}

func (h *handler) confirm(c *gin.Context) {
	var req confirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		AbortWithError(c, domain.InvalidInput("Request body must be valid JSON with reservation_id.", nil))
		return
	}

	reservation, err := h.inventory.Confirm(c.Request.Context(), domain.ConfirmInput{ReservationID: req.ReservationID})
	if err != nil {
		AbortWithError(c, err)
		return
	}

	var confirmedAt time.Time
	if reservation.ConfirmedAt != nil {
		confirmedAt = *reservation.ConfirmedAt
	}

	c.JSON(http.StatusOK, confirmResponse{
		Status:        "success",
		ReservationID: reservation.ReservationID,
		ConfirmedAt:   confirmedAt,
	})
}

type stockResponse struct {
	ItemID         string `json:"item_id"`
	TotalStock     int32  `json:"total_stock"`
	ReservedStock  int32  `json:"reserved_stock"`
	AvailableStock int32  `json:"available_stock"`
}

func (h *handler) stock(c *gin.Context) {
	stock, err := h.inventory.GetStock(c.Request.Context(), c.Query("item_id"))
	if err != nil {
		AbortWithError(c, err)
		return
	}

	c.JSON(http.StatusOK, stockResponse{
		ItemID:         stock.ItemID,
		TotalStock:     stock.TotalStock,
		ReservedStock:  stock.ReservedStock,
		AvailableStock: stock.AvailableStock,
	})
}
