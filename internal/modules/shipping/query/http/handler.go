package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/query"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Handler struct {
	reader *query.Reader
}

func New(reader *query.Reader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup) {
	authed.HandleFunc("GET /orders/{id}/shipping", h.get)
}

// Declared here, not shared with the admin slices. Three slices will hold an
// identical struct at first; that is deliberate, so one endpoint's new field
// cannot appear in another's response.
type shipmentResponse struct {
	ID             uuid.UUID             `json:"id"`
	OrderID        uuid.UUID             `json:"order_id"`
	Carrier        string                `json:"carrier,omitempty"`
	TrackingNumber string                `json:"tracking_number,omitempty"`
	Status         domain.ShipmentStatus `json:"status"`
	ShippedAt      *time.Time            `json:"shipped_at,omitempty"`
	DeliveredAt    *time.Time            `json:"delivered_at,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

func toShipmentResponse(s *domain.Shipment) shipmentResponse {
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

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	shipment, err := h.reader.GetByOrderIDForUser(r.Context(), uc.UserID, orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
