package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service *shipping.Service
	orders  shipping.OrderProvider
}

// Mirrors shipping.Shipment 1:1: nothing on it is internal or sensitive. Shared
// by GetShipping and every admin_handler.go method returning a shipment.
type shipmentResponse struct {
	ID             uuid.UUID               `json:"id"`
	OrderID        uuid.UUID               `json:"order_id"`
	Carrier        string                  `json:"carrier,omitempty"`
	TrackingNumber string                  `json:"tracking_number,omitempty"`
	Status         shipping.ShipmentStatus `json:"status"`
	ShippedAt      *time.Time              `json:"shipped_at,omitempty"`
	DeliveredAt    *time.Time              `json:"delivered_at,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

func toShipmentResponse(s *shipping.Shipment) shipmentResponse {
	return shipmentResponse{
		ID:             s.ID,
		OrderID:        s.OrderID,
		Carrier:        s.Carrier,
		TrackingNumber: s.TrackingNumber,
		Status:         s.Status,
		ShippedAt:      s.ShippedAt,
		DeliveredAt:    s.DeliveredAt,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

func (h *handler) GetShipping(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	order, err := h.orders.GetByID(r.Context(), orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	if order.UserID != uc.UserID {
		response.HandleErr(w, apperror.ErrNotFound)
		return
	}

	shipment, err := h.service.GetByOrderID(r.Context(), orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
